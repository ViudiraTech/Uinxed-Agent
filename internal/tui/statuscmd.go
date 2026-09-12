package tui

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	ctxutil "github.com/ViudiraTech/Uinxed-Agent/internal/context"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

// statusCommandInterval is deliberately slow: the command is a user script and
// its output is decorative, so it must never compete with streaming renders.
const statusCommandInterval = 5 * time.Second

type statusCmdTickMsg struct{}

type statusCmdMsg struct {
	text string
	err  error
}

func statusTick() tea.Cmd {
	return tea.Tick(statusCommandInterval, func(time.Time) tea.Msg { return statusCmdTickMsg{} })
}

// statusPayload is the JSON handed to the user's statusline command on stdin.
type statusPayload struct {
	Model        string `json:"model"`
	CWD          string `json:"cwd"`
	SessionID    string `json:"session_id"`
	SessionName  string `json:"session_name"`
	Agent        string `json:"agent"`
	Provider     string `json:"provider"`
	Effort       string `json:"effort"`
	Busy         bool   `json:"busy"`
	ContextUsed  int    `json:"context_used"`
	ContextLimit int    `json:"context_limit"`
	ContextPct   int    `json:"context_pct"`
}

func (m *Model) runStatusCommand() tea.Cmd {
	command := strings.TrimSpace(m.cfg.StatuslineCommand)
	if command == "" {
		return nil
	}
	limit := ctxutil.Window(m.session.Model)
	used := ctxutil.EstimateMessages(m.session.Messages)
	pct := 0
	if limit > 0 {
		pct = used * 100 / limit
	}
	payload, _ := json.Marshal(statusPayload{
		Model: m.session.Model, CWD: m.session.CWD,
		SessionID: m.session.ID, SessionName: m.session.Name,
		Agent: m.session.AgentID, Provider: m.session.ProviderID,
		Effort: m.currentEffort(), Busy: m.busy,
		ContextUsed: used, ContextLimit: limit, ContextPct: pct,
	})
	parent := m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd", "/C", command)
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-c", command)
		}
		cmd.Stdin = strings.NewReader(string(payload))
		out, err := cmd.Output()
		if err != nil {
			return statusCmdMsg{err: err}
		}
		return statusCmdMsg{text: firstLine(terminalutil.SanitizeText(string(out)))}
	}
}
