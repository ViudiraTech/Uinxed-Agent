// Package approval decides whether a tool call may run, must be confirmed by
// the user, or is refused outright.
//
// It is a pure policy layer: no TUI, no runtime, no I/O. The only dependency is
// tools.Category so the matrix below cannot drift from the tool registry. That
// keeps the whole decision table reachable from a table-driven unit test and
// keeps the blocking/broker concerns in internal/agent where they belong.
package approval

import "github.com/ViudiraTech/Uinxed-Agent/internal/tools"

type Mode string

const (
	// ModePlan is strictly read-only with no approval escape hatch: every
	// mutating call is refused so the model has to produce a plan instead of
	// making changes.
	ModePlan Mode = "plan"
	// ModeReadOnly allows investigation freely but asks before anything that
	// changes state, touches the network, or delegates.
	ModeReadOnly Mode = "read-only"
	// ModeAutoEdit is the default: file edits and delegations are trusted,
	// shell and network still ask.
	ModeAutoEdit Mode = "auto-edit"
	// ModeFullAuto trusts everything.
	ModeFullAuto Mode = "full-auto"
)

// DefaultMode is the fallback for an empty or unrecognised mode string. It is
// deliberately the same value config.validate falls back to.
const DefaultMode = ModeAutoEdit

type Verdict int

const (
	Allow Verdict = iota
	Ask
	Deny
)

func (v Verdict) String() string {
	switch v {
	case Allow:
		return "allow"
	case Ask:
		return "ask"
	case Deny:
		return "deny"
	}
	return "unknown"
}

// Modes is the ordered mode ring cycled by Shift+Tab.
func Modes() []Mode { return []Mode{ModePlan, ModeReadOnly, ModeAutoEdit, ModeFullAuto} }

func ValidMode(m Mode) bool {
	for _, x := range Modes() {
		if x == m {
			return true
		}
	}
	return false
}

// Normalize maps an arbitrary string to a valid mode, falling back to the
// default. Empty strings normalize to the default rather than to an error so a
// session created before this feature existed keeps working.
func Normalize(m string) Mode {
	v := Mode(m)
	if ValidMode(v) {
		return v
	}
	return DefaultMode
}

// Next returns the following mode in the ring.
func Next(m Mode) Mode {
	ring := Modes()
	cur := Normalize(string(m))
	for i, x := range ring {
		if x == cur {
			return ring[(i+1)%len(ring)]
		}
	}
	return DefaultMode
}

// Evaluate resolves the verdict for one tool call.
//
// An explicit per-session grant for this exact tool name is checked before the
// mode matrix. Grants are keyed by tool name rather than by category on
// purpose: "always allow Bash" must never silently become "always allow every
// shell tool", and the write category covers both editing one file and
// deleting a tree.
//
// The matrix itself:
//
//	mode       read/state   write          shell          network        delegate
//	plan       Allow        Deny           Deny           Deny           Deny
//	read-only  Allow        Ask            Ask            Ask            Ask
//	auto-edit  Allow        Allow          Ask            Ask            Allow
//	full-auto  Allow        Allow          Allow          Allow          Allow
//
// Every denial or prompt carries a human-readable reason so the UI can show
// why, and so a denial handed back to the model explains itself.
func Evaluate(mode Mode, tool string, cat tools.Category, sessionAlways map[string]struct{}) (Verdict, string) {
	if _, ok := sessionAlways[tool]; ok {
		return Allow, "allowed for this session"
	}
	switch Normalize(string(mode)) {
	case ModePlan:
		// No approval path exists in plan mode: even a user willing to confirm
		// must switch modes first, which is what forces a plan out of the model.
		if readOnly(cat) {
			return Allow, ""
		}
		return Deny, "plan mode is read-only; switch modes to make changes"
	case ModeReadOnly:
		if readOnly(cat) {
			return Allow, ""
		}
		return Ask, "read-only mode requires confirmation"
	case ModeAutoEdit:
		switch cat {
		case tools.CategoryRead, tools.CategoryState, tools.CategoryWrite, tools.CategoryDelegate:
			return Allow, ""
		default:
			return Ask, "auto-edit mode requires confirmation for " + string(cat) + " tools"
		}
	case ModeFullAuto:
		return Allow, ""
	}
	// Unreachable while Normalize is total, but a policy function must fail
	// closed rather than open.
	return Ask, "unrecognised mode requires confirmation"
}

func readOnly(cat tools.Category) bool {
	return cat == tools.CategoryRead || cat == tools.CategoryState
}
