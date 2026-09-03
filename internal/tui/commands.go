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
	{"/help", "显示命令与快捷键", "?"},
	{"/sidebar", "切换左侧边栏显示/隐藏", "Ctrl+B"},
	{"/connect", "接入 OpenAI-compatible Provider", ""},
	{"/provider", "查看/切换 Provider", ""},
	{"/key", "设置当前 Provider API Key", ""},
	{"/model", "查看/切换模型", ""},
	{"/thinking", "开关 reasoning/thinking 展示", "Ctrl+T"},
	{"/effort", "reasoning effort: low..max / supercode", ""},
	{"/agent", "查看/切换主 Agent", "Tab"},
	{"/quota", "查询本地网关账户/余额", ""},
	{"/context", "查看上下文占用与压缩阈值", ""},
	{"/compact", "立即压缩当前上下文", ""},
	{"/todos", "查看 Todo", "Ctrl+O"},
	{"/cd", "切换工作目录", ""},
	{"/pwd", "显示工作目录", ""},
	{"/new", "新建 Session", ""},
	{"/sessions", "切换 Session", ""},
	{"/rename", "重命名当前 Session", ""},
	{"/parent", "返回父 Agent Session", ""},
	{"/delete", "删除 Session", ""},
	{"/storage", "SQLite/config.json 存储互转", ""},
	{"/migrate", "等价 /storage", ""},
	{"/diff", "打开 Git Diff 审阅器", "Ctrl+D"},
	{"/skills", "查看/加载 Agent Skill", ""},
	{"/mouse", "开关鼠标捕获", ""},
	{"/theme", "切换主题 (uinxed/tokyonight/catppuccin/dark/light)", ""},
	{"/clear", "清空当前 Session", ""},
	{"/restore", "恢复出厂设置", ""},
	{"/exit", "退出", "Ctrl+C"},
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
		var b strings.Builder
		for _, c := range commandDefs {
			fmt.Fprintf(&b, "%-12s %s", c.Name, c.Desc)
			if c.Shortcut != "" {
				fmt.Fprintf(&b, "  [%s]", c.Shortcut)
			}
			b.WriteByte('\n')
		}
		b.WriteString("\n快捷键: Ctrl+P command palette · Ctrl+T reasoning · Ctrl+O todos · Ctrl+E tool details · PgUp/PgDn scroll · Esc cancel")
		m.openInfo("Help", strings.TrimSpace(b.String()), overlayHelp)
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
			shown := "未设置"
			if key != "" {
				shown = config.RedactSecret(key)
			}
			m.openInfo("API Key · "+p.Name, shown+"\n\n设置: /key <key>", overlayInfo)
			return nil
		}
		key := strings.TrimSpace(arg)
		pid := m.session.ProviderID
		return asyncOp("key", func() (any, error) {
			if err := m.ctrl.CheckKey(m.ctx, pid, key); err != nil {
				return nil, fmt.Errorf("Key 验证失败: %w", err)
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
		m.openInfo("Context", fmt.Sprintf("模型: %s\n窗口: %d tokens\n已用: ≈%d tokens (%d%%)\n自动压缩阈值: ≈%d tokens (62%%)\n请求历史预算: ≈%d tokens (72%%)", m.session.Model, window, used, pct, thr, ctxutil.HistoryBudget(m.session.Model)), overlayContext)
	case "/compact":
		if m.busy {
			m.showToast("当前 Agent 正在运行")
			return nil
		}
		return asyncOp("compact", func() (any, error) { return nil, m.ctrl.Compact(m.ctx, sid) })
	case "/todos":
		m.overlay = overlayTodos
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
			name = fmt.Sprintf("会话 %d", len(m.sessions)+1)
		}
		return asyncOp("new_session", func() (any, error) { return m.ctrl.NewSession(m.ctx, name) })
	case "/sessions":
		m.openSessionPicker()
	case "/rename":
		newName := strings.TrimSpace(arg)
		if newName == "" {
			m.openInfo("Rename Session", "用法: /rename <新名称>", overlayInfo)
			return nil
		}
		return asyncOp("rename", func() (any, error) { return nil, m.ctrl.RenameSession(m.ctx, sid, newName) })
	case "/parent":
		if m.session.ParentID == "" {
			m.showToast("当前已经是主 Session")
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
			m.showError(fmt.Errorf("没有会话: %s", arg))
			return nil
		}
		m.confirmTarget = target.ID
		m.infoText = target.Name
		m.overlay = overlayConfirmDelete
		m.setFocus(FocusOverlay)
	case "/storage", "/migrate":
		target := strings.ToLower(strings.TrimSpace(arg))
		if target == "" {
			m.openInfo("Storage", fmt.Sprintf("当前: %s\n\n/storage db      迁入 SQLite\n/storage config  写回 config.json", m.ctrl.Config.Snapshot().Storage), overlayInfo)
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
		if v != "uinxed" && v != "dark" && v != "light" && v != "tokyonight" && v != "catppuccin" {
			m.showError(fmt.Errorf("theme must be uinxed, tokyonight, catppuccin, dark, or light"))
			return nil
		}
		return asyncOp("theme", func() (any, error) {
			return v, m.ctrl.Config.Update(func(c *config.Config) error { c.Theme = v; return nil })
		})
	case "/clear":
		if m.busy {
			m.showToast("先取消当前生成")
			return nil
		}
		return asyncOp("clear", func() (any, error) { return nil, m.ctrl.ClearSession(m.ctx, sid) })
	case "/restore":
		m.overlay = overlayConfirmRestore
		m.setFocus(FocusOverlay)
	default:
		m.showError(fmt.Errorf("未知命令: %s", name))
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

func (m *Model) openCommandPalette() {
	items := []PickerItem{
		{"new", "New Session", "创建新会话", ""},
		{"sessions", "Switch Session", "切换会话", ""},
		{"sidebar", "Toggle Sidebar", "切换左侧边栏", "Ctrl+B"},
		{"agent", "Change Agent", "切换 Agent", "Tab"},
		{"model", "Change Model", "切换模型", ""},
		{"provider", "Change Provider", "切换 Provider", ""},
		{"thinking", "Toggle Thinking", "切换思考过程显示", "Ctrl+T"},
		{"tools", "Toggle Tool Details", "切换工具调用详情", "Ctrl+E"},
		{"compact", "Compact Context", "压缩当前上下文", ""},
		{"todos", "Show Todos", "查看待办任务", "Ctrl+O"},
		{"diff", "Open Diff", "审阅代码改动", "Ctrl+D"},
		{"theme", "Change Theme", "切换界面颜色主题", ""},
		{"rename", "Rename Session", "重命名当前会话", ""},
		{"mouse", "Toggle Mouse", "鼠标捕获开关", ""},
		{"quit", "Quit", "退出", "Ctrl+C"},
	}
	if m.session.ParentID != "" {
		items = append(items, PickerItem{"parent", "Return to Parent", "返回父 Agent Session", "Esc/command"})
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
			d = "max reasoning + 多子 Agent 并发"
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
		{"uinxed", "Uinxed Cyberpunk", "赛博朋克紫青霓虹 (默认)"},
		{"tokyonight", "Tokyo Night", "东京之夜深蓝冷调"},
		{"catppuccin", "Catppuccin Mocha", "柔和舒适马卡龙调色"},
		{"dark", "Dark Slate", "沉稳灰阶高对比暗黑"},
		{"light", "Light Clean", "明亮清爽浅色纸张"},
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
	case "session":
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
		m.conv.ToggleAllThinking(m.streamReasoning)
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
			m.showToast("名称不能为空")
			return nil
		}
		m.connect.Name = v
		m.connect.Step = 1
	case 1:
		if v == "" {
			m.showToast("Base URL 不能为空")
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
