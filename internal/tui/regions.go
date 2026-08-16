package tui

type Rect struct{ X, Y, W, H int }

func (r Rect) Contains(x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

type ActionKind string

const (
	ActionAgent    ActionKind = "agent"
	ActionModel    ActionKind = "model"
	ActionProvider ActionKind = "provider"
	ActionSession  ActionKind = "session"
	ActionTool     ActionKind = "tool"
	ActionCommand  ActionKind = "command"
	ActionPicker   ActionKind = "picker"
	ActionDiffFile ActionKind = "diff_file"
	ActionTodo     ActionKind = "todo"
	ActionThinking ActionKind = "thinking"
	ActionSidebar  ActionKind = "sidebar"
	ActionPrompt   ActionKind = "prompt"
	ActionChat     ActionKind = "chat"
	ActionButton   ActionKind = "button"
)

type Region struct {
	Rect  Rect
	Kind  ActionKind
	Value string
}

func findRegion(rs []Region, x, y int) (Region, bool) {
	best := -1
	bestPriority := -1
	for i := range rs {
		if !rs[i].Rect.Contains(x, y) {
			continue
		}
		priority := regionPriority(rs[i].Kind)
		// Prefer specific interactive controls over broad background regions.
		// For equal priority, later regions retain z-order semantics.
		if priority >= bestPriority {
			best = i
			bestPriority = priority
		}
	}
	if best >= 0 {
		return rs[best], true
	}
	return Region{}, false
}

func regionPriority(kind ActionKind) int {
	switch kind {
	case ActionChat, ActionSidebar:
		return 0
	case ActionPrompt:
		return 1
	default:
		return 2
	}
}
