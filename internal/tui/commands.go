package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/agent"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	ctxutil "github.com/ViudiraTech/Uinxed-Agent/internal/context"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/skills"
)

type commandDef struct {
	Name     string
	Desc     string
	Shortcut string
}

var commandDefs = []commandDef{
	{"/help", "Show commands and shortcuts", "?"},
	{"/sidebar", "Toggle the session sidebar", "Ctrl+B"},
	{"/connect", "Add an OpenAI-compatible provider", ""},
	{"/provider", "Show or switch provider", ""},
	{"/key", "Set the API key for the current provider", ""},
	{"/model", "Show or switch model", ""},
	{"/thinking", "Toggle reasoning display", "Ctrl+T"},
	{"/effort", "Reasoning effort: low..max / supercode", ""},
	{"/agent", "Show or switch the primary agent", "Tab"},
	{"/quota", "Query gateway account and balance", ""},
	{"/context", "Show context usage and compaction thresholds", ""},
	{"/compact", "Compact the context now", ""},
	{"/todos", "Show the todo list", "Ctrl+O"},
	{"/plan", "Show the current plan", ""},
	{"/cd", "Change the working directory", ""},
	{"/pwd", "Print the working directory", ""},
	{"/new", "Start a new session", ""},
	{"/sessions", "Switch session", ""},
	{"/search", "Search sessions by text", ""},
	{"/export", "Export this session to Markdown", ""},
	{"/rename", "Rename the current session", ""},
	{"/parent", "Return to the parent agent session", ""},
	{"/delete", "Delete a session", ""},
	{"/storage", "Switch between SQLite and config.json storage", ""},
	{"/migrate", "Alias for /storage", ""},
	{"/diff", "Open the git diff reviewer", "Ctrl+D"},
	{"/skills", "Browse and load agent skills", ""},
	{"/mouse", "Toggle terminal mouse capture", ""},
	{"/theme", "Switch color theme", ""},
	{"/clear", "Clear the current session", ""},
	{"/restore", "Restore factory settings", ""},
	{"/exit", "Quit", "Ctrl+C"},
}

func (m *Model) executeCommand(text string) tea.Cmd {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil
	}
	name := strings.ToLower(fields[0])
	arg := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	sid := m.session.ID
	switch name {
	case "/help":
		m.openInfo("Help", helpText(), overlayHelp)
	case "/exit", "/quit":
		return tea.Quit
	case "/agent":
		if arg == "" {
			m.openAgentPicker()
			return nil
		}
		id := strings.TrimSpace(arg)
		return asyncOp("set_agent", func() (any, error) { return nil, m.ctrl.SetAgent(m.ctx, sid, id) })
	case "/model":
		if arg == "" {
			return m.fetchModels()
		}
		model := strings.TrimSpace(arg)
		return asyncOp("set_model", func() (any, error) { return nil, m.ctrl.SetModel(m.ctx, sid, model) })
	case "/provider":
		if arg == "" {
			m.openProviderPicker()
			return nil
		}
		id := strings.TrimSpace(arg)
		return asyncOp("set_provider", func() (any, error) { return nil, m.ctrl.SetProvider(m.ctx, sid, id) })
	case "/connect":
		m.connect = connectWizard{Step: 0}
		m.overlay = overlayConnect
		m.setFocus(FocusOverlay)
	case "/key":
		if arg == "" {
			p := m.ctrl.Config.ActiveProvider()
			key, _ := m.ctrl.Config.ProviderKey(p.ID)
			shown := "not set"
			if key != "" {
				shown = config.RedactSecret(key)
			}
			m.openInfo("API Key · "+p.Name, shown+"\n\nSet with: /key <key>", overlayInfo)
			return nil
		}
		key := strings.TrimSpace(arg)
		pid := m.session.ProviderID
		return asyncOp("key", func() (any, error) {
			if err := m.ctrl.CheckKey(m.ctx, pid, key); err != nil {
				return nil, fmt.Errorf("key check failed: %w", err)
			}
			return nil, m.ctrl.SetKey(pid, key)
		})
	case "/thinking":
		v, err := parseBoolWord(strings.TrimSpace(arg), m.thinkingEnabled())
		if err != nil {
			m.showError(err)
			return nil
		}
		return asyncOp("set_thinking", func() (any, error) { return nil, m.ctrl.SetThinking(m.ctx, sid, v) })
	case "/effort":
		if arg == "" {
			m.openEffortPicker()
			return nil
		}
		effort := strings.ToLower(strings.TrimSpace(arg))
		if !validEffort(effort) {
			m.showError(fmt.Errorf("effort must be low, medium, high, xhigh, max or supercode"))
			return nil
		}
		return asyncOp("set_effort", func() (any, error) { return nil, m.ctrl.SetEffort(m.ctx, sid, effort) })
	case "/quota":
		pid := m.session.ProviderID
		return asyncOp("profile", func() (any, error) { return m.ctrl.Profile(m.ctx, pid) })
	case "/context":
		window := ctxutil.Window(m.session.Model)
		used := ctxutil.EstimateMessages(m.session.Messages)
		thr := ctxutil.CompactThreshold(m.session.Model)
		pct := 0
		if window > 0 {
			pct = used * 100 / window
		}
		m.openInfo("Context", fmt.Sprintf("Model: %s\nWindow: %d tokens\nUsed: ≈%d tokens (%d%%)\nAuto-compact threshold: ≈%d tokens (62%%)\nHistory budget: ≈%d tokens (72%%)", m.session.Model, window, used, pct, thr, ctxutil.HistoryBudget(m.session.Model)), overlayContext)
	case "/compact":
		if m.busy {
			m.showToast("agent is still running")
			return nil
		}
		return asyncOp("compact", func() (any, error) { return nil, m.ctrl.Compact(m.ctx, sid) })
	case "/todos":
		m.overlay = overlayTodos
		m.overlayScroll = 0
		m.setFocus(FocusTodos)
	case "/plan":
		m.overlay = overlayPlan
		m.overlayScroll = 0
		m.setFocus(FocusTodos)
	case "/pwd":
		m.openInfo("Working Directory", m.session.CWD, overlayInfo)
	case "/cd":
		if arg == "" {
			m.openInfo("Working Directory", m.session.CWD, overlayInfo)
			return nil
		}
		next := strings.TrimSpace(arg)
		if !filepath.IsAbs(next) {
			next = filepath.Join(m.session.CWD, next)
		}
		return asyncOp("cd", func() (any, error) { return nil, m.ctrl.ChangeCWD(m.ctx, sid, next) })
	case "/new":
		name := strings.TrimSpace(arg)
		if name == "" {
			name = fmt.Sprintf("Session %d", len(m.sessions)+1)
		}
		return asyncOp("new_session", func() (any, error) { return m.ctrl.NewSession(m.ctx, name) })
	case "/sessions":
		m.openSessionPicker()
	case "/search":
		if arg == "" {
			m.openInfo("Search", "Usage: /search <query>", overlayInfo)
			return nil
		}
		q := arg
		return asyncOp("search_sessions", func() (any, error) { return m.ctrl.SearchSessions(m.ctx, q, 30) })
	case "/export":
		return asyncOp("export", func() (any, error) { return m.ctrl.ExportSession(m.ctx, sid, arg) })
	case "/rename":
		newName := strings.TrimSpace(arg)
		if newName == "" {
			m.openInfo("Rename Session", "Usage: /rename <new name>", overlayInfo)
			return nil
		}
		return asyncOp("rename", func() (any, error) { return nil, m.ctrl.RenameSession(m.ctx, sid, newName) })
	case "/parent":
		if m.session.ParentID == "" {
			m.showToast("already at the root session")
			return nil
		}
		return m.switchSessionCmd(m.session.ParentID)
	case "/delete":
		if arg == "" {
			m.openDeletePicker()
			return nil
		}
		target := m.resolveSession(strings.TrimSpace(arg))
		if target == nil {
			m.showError(fmt.Errorf("no such session: %s", arg))
			return nil
		}
		m.confirmTarget = target.ID
		m.infoText = target.Name
		m.overlay = overlayConfirmDelete
		m.setFocus(FocusOverlay)
	case "/storage", "/migrate":
		target := strings.ToLower(strings.TrimSpace(arg))
		if target == "" {
			m.openInfo("Storage", fmt.Sprintf("Current: %s\n\n/storage db      migrate into SQLite\n/storage config  write back to config.json", m.ctrl.Config.Snapshot().Storage), overlayInfo)
			return nil
		}
		return asyncOp("storage", func() (any, error) { n, e := m.ctrl.SwitchStorage(m.ctx, target); return n, e })
	case "/diff":
		cwd := m.session.CWD
		return func() tea.Msg { s, e := m.ctrl.Diff(m.ctx, cwd); return diffMsg{s, e} }
	case "/skills":
		if arg == "" {
			m.openSkillPicker()
			return nil
		}
		sk, ok, err := skills.Get(strings.TrimSpace(arg), m.session.CWD)
		if err != nil {
			m.showError(err)
			return nil
		}
		if !ok {
			m.showError(fmt.Errorf("skill not found: %s", arg))
			return nil
		}
		m.openInfo("Skill · "+sk.Name, sk.Body, overlayInfo)
	case "/mouse":
		cur := m.ctrl.Config.Snapshot().Mouse
		v, err := parseBoolWord(strings.TrimSpace(arg), cur)
		if err != nil {
			m.showError(err)
			return nil
		}
		return asyncOp("mouse", func() (any, error) {
			return v, m.ctrl.Config.Update(func(c *config.Config) error { c.Mouse = v; return nil })
		})
	case "/sidebar":
		return m.toggleSidebar()
	case "/theme":
		v := strings.ToLower(strings.TrimSpace(arg))
		if v == "" {
			m.openThemePicker()
			return nil
		}
		if !config.ValidTheme(v) {
			m.showError(fmt.Errorf("theme must be one of: %s", strings.Join(config.Themes(), ", ")))
			return nil
		}
		return asyncOp("theme", func() (any, error) {
			return v, m.ctrl.Config.Update(func(c *config.Config) error { c.Theme = v; return nil })
		})
	case "/clear":
		if m.busy {
			m.showToast("cancel the running turn first")
			return nil
		}
		return asyncOp("clear", func() (any, error) { return nil, m.ctrl.ClearSession(m.ctx, sid) })
	case "/restore":
		m.overlay = overlayConfirmRestore
		m.setFocus(FocusOverlay)
	default:
		m.showError(fmt.Errorf("unknown command: %s", name))
	}
	return nil
}

func validEffort(v string) bool {
	switch v {
	case "low", "medium", "high", "xhigh", "max", "supercode":
		return true
	}
	return false
}

// helpText renders the grouped shortcuts and command reference shown by /help.
func helpText() string {
	var b strings.Builder
	b.WriteString("Shortcuts\n")
	for _, s := range [][2]string{
		{"Ctrl+P", "Command palette"},
		{"Tab", "Accept completion, else cycle agent"},
		{"Ctrl+O", "Show todos"},
		{"Ctrl+T", "Expand or collapse reasoning"},
		{"Ctrl+E", "Expand or collapse tool details"},
		{"Ctrl+B", "Toggle sidebar"},
		{"Ctrl+D", "Open git diff"},
		{"Ctrl+R", "Search prompt history"},
		{"Shift+Tab", "Cycle approval mode"},
		{"PgUp/PgDn", "Scroll the conversation"},
		{"Esc", "Close overlay; cancel the running turn"},
		{"Ctrl+C", "Cancel the running turn; quit when idle"},
		{"@file", "Attach file context to the turn"},
		{"@agent task", "Delegate to explorer, coding or general"},
		{"?", "This panel"},
	} {
		fmt.Fprintf(&b, "  %-13s %s\n", s[0], s[1])
	}
	b.WriteString("\nCommands\n")
	for _, c := range commandDefs {
		fmt.Fprintf(&b, "  %-11s %s", c.Name, c.Desc)
		if c.Shortcut != "" {
			fmt.Fprintf(&b, "  [%s]", c.Shortcut)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) openCommandPalette() {
	items := []PickerItem{
		{"new", "New Session", "Start a fresh conversation", ""},
		{"sessions", "Switch Session", "Jump to another session", ""},
		{"search", "Search Sessions", "Find a session by name or message text", ""},
		{"export", "Export Session", "Write this conversation as Markdown", ""},
		{"sidebar", "Toggle Sidebar", "Show or hide the session sidebar", "Ctrl+B"},
		{"agent", "Change Agent", "Switch the primary agent", "Tab"},
		{"model", "Change Model", "Switch the active model", ""},
		{"provider", "Change Provider", "Switch the active provider", ""},
		{"thinking", "Toggle Thinking", "Show or hide reasoning", "Ctrl+T"},
		{"tools", "Toggle Tool Details", "Expand or collapse tool output", "Ctrl+E"},
		{"compact", "Compact Context", "Summarize the context now", ""},
		{"todos", "Show Todos", "Review the task list", "Ctrl+O"},
		{"diff", "Open Diff", "Review working tree changes", "Ctrl+D"},
		{"theme", "Change Theme", "Pick a color theme", ""},
		{"rename", "Rename Session", "Give the current session a new name", ""},
		{"mouse", "Toggle Mouse", "Turn terminal mouse capture on or off", ""},
		{"quit", "Quit", "Exit ux-agent", "Ctrl+C"},
	}
	if m.session.ParentID != "" {
		items = append(items, PickerItem{"parent", "Return to Parent", "Go back to the parent agent session", ""})
	}
	m.picker.Reset("Command Palette", ActionCommand, items)
	m.pickerPurpose = "command"
	m.overlay = overlayPicker
	m.setFocus(FocusCommandPalette)
}
func (m *Model) openAgentPicker() {
	var items []PickerItem
	for _, a := range agent.Primary() {
		items = append(items, PickerItem{a.ID, a.Name, a.Description, ""})
	}
	m.picker.Reset("Agent", ActionAgent, items)
	m.pickerPurpose = "agent"
	m.overlay = overlayPicker
	m.setFocus(FocusPicker)
}
func (m *Model) openModelPicker(models []string) {
	if len(models) == 0 {
		models = m.ctrl.Config.ActiveProvider().Models
	}
	items := make([]PickerItem, 0, len(models))
	for _, x := range models {
		items = append(items, PickerItem{x, x, "", ""})
	}
	m.picker.Reset("Model", ActionModel, items)
	m.pickerPurpose = "model"
	m.overlay = overlayPicker
	m.setFocus(FocusPicker)
}
func (m *Model) openProviderPicker() {
	cfg := m.ctrl.Config.Snapshot()
	var items []PickerItem
	for _, p := range cfg.Providers {
		desc := p.BaseURL
		if p.ID == cfg.ActiveProvider {
			desc = "current · " + desc
		}
		items = append(items, PickerItem{p.ID, p.Name, desc, ""})
	}
	m.picker.Reset("Provider", ActionProvider, items)
	m.pickerPurpose = "provider"
	m.overlay = overlayPicker
	m.setFocus(FocusPicker)
}
func (m *Model) openEffortPicker() {
	vals := []string{"low", "medium", "high", "xhigh", "max", "supercode"}
	var items []PickerItem
	for _, v := range vals {
		d := ""
		if v == "supercode" {
			d = "max reasoning with concurrent subagents"
		}
		items = append(items, PickerItem{v, v, d, ""})
	}
	m.picker.Reset("Reasoning Effort", ActionButton, items)
	m.pickerPurpose = "effort"
	m.overlay = overlayPicker
	m.setFocus(FocusPicker)
}
func (m *Model) openSessionPicker() {
	var items []PickerItem
	for _, s := range m.sessions {
		desc := s.AgentID + " · " + formatAgo(s.UpdatedAt)
		if s.ID == m.session.ID {
			desc = "current · " + desc
		}
		items = append(items, PickerItem{s.ID, s.Name, desc, ""})
	}
	m.picker.Reset("Sessions", ActionSession, items)
	m.pickerPurpose = "session"
	m.overlay = overlayPicker
	m.setFocus(FocusPicker)
}
func (m *Model) openDeletePicker() {
	var items []PickerItem
	for _, s := range m.sessions {
		items = append(items, PickerItem{s.ID, s.Name, "delete · " + formatAgo(s.UpdatedAt), ""})
	}
	m.picker.Reset("Delete Session", ActionSession, items)
	m.pickerPurpose = "delete"
	m.overlay = overlayPicker
	m.setFocus(FocusPicker)
}
func (m *Model) openSkillPicker() {
	ss, _ := skills.List(m.session.CWD)
	var items []PickerItem
	for _, s := range ss {
		items = append(items, PickerItem{s.Name, s.Name, s.Description, ""})
	}
	m.picker.Reset("Skills", ActionButton, items)
	m.pickerPurpose = "skill"
	m.overlay = overlayPicker
	m.setFocus(FocusPicker)
}
func (m *Model) openThemePicker() {
	opts := []struct {
		id, label, desc string
	}{
		{"uinxed", "Uinxed Cyberpunk", "Purple and cyan neon (default)"},
		{"tokyonight", "Tokyo Night", "Cool deep blues"},
		{"catppuccin", "Catppuccin Mocha", "Soft pastel palette"},
		{"gruvbox", "Gruvbox", "Warm retro earth tones"},
		{"nord", "Nord", "Muted arctic blues"},
		{"dracula", "Dracula", "High-contrast purple"},
		{"dark", "Dark Slate", "Neutral high-contrast dark"},
		{"light", "Light Clean", "Bright paper-like light"},
	}
	var items []PickerItem
	for _, x := range opts {
		items = append(items, PickerItem{ID: x.id, Label: x.label, Description: x.desc})
	}
	m.picker.Reset("Select Theme", ActionButton, items)
	m.pickerPurpose = "theme"
	m.overlay = overlayPicker
	m.setFocus(FocusPicker)
}

func (m *Model) handlePickerKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.closeOverlay()
	case "up", "ctrl+k":
		m.picker.Move(-1)
	case "down", "ctrl+j":
		m.picker.Move(1)
	case "enter":
		return m.choosePicker()
	case "backspace":
		m.picker.SetQuery(removeLastRune(m.picker.Query))
	default:
		if k.Text != "" {
			m.picker.SetQuery(m.picker.Query + k.Text)
		}
	}
	return nil
}
func (m *Model) choosePicker() tea.Cmd {
	it, ok := m.picker.Selected()
	if !ok {
		return nil
	}
	purpose := m.pickerPurpose
	m.closeOverlay()
	sid := m.session.ID
	switch purpose {
	case "agent":
		return asyncOp("set_agent", func() (any, error) { return nil, m.ctrl.SetAgent(m.ctx, sid, it.ID) })
	case "model":
		return asyncOp("set_model", func() (any, error) { return nil, m.ctrl.SetModel(m.ctx, sid, it.ID) })
	case "provider":
		return asyncOp("set_provider", func() (any, error) { return nil, m.ctrl.SetProvider(m.ctx, sid, it.ID) })
	case "session", "search":
		return m.switchSessionCmd(it.ID)
	case "delete":
		m.confirmTarget = it.ID
		m.infoText = it.Label
		m.overlay = overlayConfirmDelete
		m.setFocus(FocusOverlay)
		return nil
	case "effort":
		return asyncOp("set_effort", func() (any, error) { return nil, m.ctrl.SetEffort(m.ctx, sid, it.ID) })
	case "skill":
		sk, ok, e := skills.Get(it.ID, m.session.CWD)
		if e != nil {
			m.showError(e)
		} else if ok {
			m.openInfo("Skill · "+sk.Name, sk.Body, overlayInfo)
		}
		return nil
	case "theme":
		return asyncOp("theme", func() (any, error) {
			return it.ID, m.ctrl.Config.Update(func(c *config.Config) error { c.Theme = it.ID; return nil })
		})
	case "command":
		return m.runPaletteAction(it.ID)
	}
	return nil
}
func (m *Model) runPaletteAction(id string) tea.Cmd {
	switch id {
	case "new":
		return m.executeCommand("/new")
	case "sessions":
		m.openSessionPicker()
	case "search":
		m.prompt.SetValue("/search ")
		m.closeOverlay()
	case "export":
		return m.executeCommand("/export")
	case "rename":
		m.prompt.SetValue("/rename ")
		m.closeOverlay()
	case "parent":
		return m.executeCommand("/parent")
	case "agent":
		m.openAgentPicker()
	case "model":
		return m.fetchModels()
	case "provider":
		m.openProviderPicker()
	case "compact":
		return m.executeCommand("/compact")
	case "sidebar":
		return m.toggleSidebar()
	case "thinking":
		m.conv.ToggleAllThinking()
		m.closeOverlay()
		return nil
	case "tools":
		m.conv.ToggleAllTools()
		m.closeOverlay()
		return nil
	case "todos":
		m.overlay = overlayTodos
		m.overlayScroll = 0
		m.setFocus(FocusTodos)
	case "diff":
		return m.executeCommand("/diff")
	case "theme":
		m.openThemePicker()
	case "mouse":
		return m.executeCommand("/mouse")
	case "quit":
		return tea.Quit
	}
	return nil
}

func (m *Model) cycleAgent() tea.Cmd {
	ps := agent.Primary()
	if len(ps) == 0 {
		return nil
	}
	idx := 0
	for i, a := range ps {
		if a.ID == m.session.AgentID {
			idx = i
			break
		}
	}
	next := ps[(idx+1)%len(ps)].ID
	return asyncOp("set_agent", func() (any, error) { return nil, m.ctrl.SetAgent(m.ctx, m.session.ID, next) })
}

func (m *Model) updateInlineSuggestions() {
	value := m.prompt.Value()
	m.commandMatches = nil
	m.atMatches = nil
	if strings.HasPrefix(value, "/") && !strings.Contains(value, " ") {
		q := strings.ToLower(value)
		for _, c := range commandDefs {
			if strings.HasPrefix(c.Name, q) {
				m.commandMatches = append(m.commandMatches, PickerItem{c.Name, c.Name, c.Desc, c.Shortcut})
				if len(m.commandMatches) >= 8 {
					break
				}
			}
		}
		return
	}
	at := strings.LastIndex(value, "@")
	if at < 0 {
		return
	}
	token := value[at+1:]
	if strings.ContainsAny(token, " \n\t") {
		return
	}
	q := strings.ToLower(token)
	for _, a := range agent.Subagents() {
		if q == "" || strings.Contains(a.ID, q) {
			m.atMatches = append(m.atMatches, PickerItem{"agent:" + a.ID, "@" + a.ID, a.Description, ""})
		}
	}
	if m.ctrl.Index != nil {
		for _, x := range m.ctrl.Index.Search(token, 12) {
			m.atMatches = append(m.atMatches, PickerItem{"file:" + x.Path, "@" + x.Path, "file", ""})
		}
	}
	ss, _ := skills.List(m.session.CWD)
	for _, s := range ss {
		if q == "" || strings.Contains(strings.ToLower(s.Name), q) {
			m.atMatches = append(m.atMatches, PickerItem{"skill:" + s.Name, "@skill:" + s.Name, s.Description, ""})
		}
	}
	if len(m.atMatches) > 12 {
		m.atMatches = m.atMatches[:12]
	}
}
func (m *Model) acceptCommandSuggestion() tea.Cmd {
	if len(m.commandMatches) == 0 {
		return nil
	}
	m.prompt.SetValue(m.commandMatches[0].ID + " ")
	m.commandMatches = nil
	return nil
}
func (m *Model) acceptAutocomplete() tea.Cmd {
	if len(m.atMatches) == 0 {
		return nil
	}
	v := m.prompt.Value()
	at := strings.LastIndex(v, "@")
	if at < 0 {
		return nil
	}
	replacement := m.atMatches[0].Label
	m.prompt.SetValue(v[:at] + replacement + " ")
	m.atMatches = nil
	return nil
}

func (m *Model) handleConnectKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == "esc" {
		m.closeOverlay()
		return nil
	}
	if key == "backspace" {
		m.connect.Input = removeLastRune(m.connect.Input)
		return nil
	}
	if key != "enter" {
		if k.Text != "" {
			m.connect.Input += k.Text
		}
		return nil
	}
	v := strings.TrimSpace(m.connect.Input)
	m.connect.Input = ""
	switch m.connect.Step {
	case 0:
		if v == "" {
			m.showToast("name cannot be empty")
			return nil
		}
		m.connect.Name = v
		m.connect.Step = 1
	case 1:
		if v == "" {
			m.showToast("base URL cannot be empty")
			return nil
		}
		m.connect.BaseURL = strings.TrimRight(v, "/")
		m.connect.Step = 2
	case 2:
		m.connect.Models = v
		m.connect.Step = 3
	case 3:
		m.connect.Key = v
		p := config.Provider{Name: m.connect.Name, BaseURL: m.connect.BaseURL, Models: splitCSV(m.connect.Models)}
		if len(p.Models) == 0 {
			p.Models = []string{"default"}
		}
		p.DefaultModel = p.Models[0]
		return asyncOp("connect", func() (any, error) {
			if err := m.ctrl.Config.UpsertProvider(p, m.connect.Key); err != nil {
				return nil, err
			}
			cfg := m.ctrl.Config.Snapshot()
			id := ""
			for _, x := range cfg.Providers {
				if x.Name == p.Name && x.BaseURL == p.BaseURL {
					id = x.ID
					break
				}
			}
			if id == "" {
				return nil, fmt.Errorf("provider created but could not resolve id")
			}
			if m.connect.Key != "" {
				if err := m.ctrl.CheckKey(m.ctx, id, m.connect.Key); err != nil {
					return nil, err
				}
			}
			return id, m.ctrl.SetProvider(m.ctx, m.session.ID, id)
		})
	}
	return nil
}
func splitCSV(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		x = strings.TrimSpace(x)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func (m *Model) handleConfirmKey(k tea.KeyPressMsg) tea.Cmd {
	switch strings.ToLower(k.String()) {
	case "esc", "n":
		m.closeOverlay()
		return nil
	case "enter", "y":
		if m.overlay == overlayConfirmRestore {
			return asyncOp("reset", func() (any, error) { return nil, m.ctrl.Reset(m.ctx) })
		}
		if m.overlay == overlayConfirmDelete {
			id := m.confirmTarget
			return asyncOp("delete", func() (any, error) { return nil, m.ctrl.DeleteSession(m.ctx, id) })
		}
	}
	return nil
}

func (m *Model) resolveSession(v string) *domain.Session {
	if n := atoi(v); n > 0 && n <= len(m.sessions) {
		return &m.sessions[n-1]
	}
	for i := range m.sessions {
		if m.sessions[i].ID == v || m.sessions[i].Name == v {
			return &m.sessions[i]
		}
	}
	return nil
}

func (m *Model) toggleSidebar() tea.Cmd {
	next := "off"
	if m.cfg.Sidebar == "off" {
		next = "on"
	}
	m.cfg.Sidebar = next
	m.resize()
	return asyncOp("sidebar", func() (any, error) {
		return next, m.ctrl.Config.Update(func(c *config.Config) error {
			c.Sidebar = next
			return nil
		})
	})
}
