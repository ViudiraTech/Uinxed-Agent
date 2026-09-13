package approval

import (
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/tools"
)

// evalCase is one cell of the decision matrix. The table below is generated
// from the mode x category product so every combination is covered, including
// the ones the shipped UI cannot currently reach.
type evalCase struct {
	mode    Mode
	cat     tools.Category
	want    Verdict
	because string
}

var allCategories = []tools.Category{
	tools.CategoryRead,
	tools.CategoryState,
	tools.CategoryWrite,
	tools.CategoryShell,
	tools.CategoryNetwork,
	tools.CategoryDelegate,
}

// matrix is the expected verdict per mode and category, written out longhand
// rather than derived, so a change to Evaluate has to be matched by a change
// here instead of silently agreeing with itself.
var matrix = map[Mode]map[tools.Category]Verdict{
	ModePlan: {
		tools.CategoryRead: Allow, tools.CategoryState: Allow,
		tools.CategoryWrite: Deny, tools.CategoryShell: Deny,
		tools.CategoryNetwork: Deny, tools.CategoryDelegate: Deny,
	},
	ModeReadOnly: {
		tools.CategoryRead: Allow, tools.CategoryState: Allow,
		tools.CategoryWrite: Ask, tools.CategoryShell: Ask,
		tools.CategoryNetwork: Ask, tools.CategoryDelegate: Ask,
	},
	ModeAutoEdit: {
		tools.CategoryRead: Allow, tools.CategoryState: Allow,
		tools.CategoryWrite: Allow, tools.CategoryShell: Ask,
		tools.CategoryNetwork: Ask, tools.CategoryDelegate: Allow,
	},
	ModeFullAuto: {
		tools.CategoryRead: Allow, tools.CategoryState: Allow,
		tools.CategoryWrite: Allow, tools.CategoryShell: Allow,
		tools.CategoryNetwork: Allow, tools.CategoryDelegate: Allow,
	},
}

func TestEvaluateMatrix(t *testing.T) {
	for _, mode := range Modes() {
		want, ok := matrix[mode]
		if !ok {
			t.Fatalf("mode %q has no expected row in the matrix table", mode)
		}
		for _, cat := range allCategories {
			expected, ok := want[cat]
			if !ok {
				t.Fatalf("mode %q category %q has no expected verdict", mode, cat)
			}
			got, reason := Evaluate(mode, "some_tool", cat, nil)
			if got != expected {
				t.Errorf("Evaluate(%q, %q) = %v (%s), want %v", mode, cat, got, reason, expected)
			}
			// A verdict that is not a plain allow must explain itself; the UI
			// renders the reason and the model receives it on denial.
			if expected != Allow && reason == "" {
				t.Errorf("Evaluate(%q, %q) returned %v with an empty reason", mode, cat, got)
			}
			if expected == Allow && reason != "" {
				t.Errorf("Evaluate(%q, %q) allowed with a non-empty reason %q", mode, cat, reason)
			}
		}
	}
}

func TestAllModesAndCategoriesAreCovered(t *testing.T) {
	if len(matrix) != len(Modes()) {
		t.Fatalf("matrix covers %d modes, ring has %d", len(matrix), len(Modes()))
	}
	for _, cat := range allCategories {
		for _, mode := range Modes() {
			if _, ok := matrix[mode][cat]; !ok {
				t.Fatalf("missing matrix cell for %q/%q", mode, cat)
			}
		}
	}
}

// TestSessionAlwaysOverridesMatrix pins the ordering contract: the grant table
// is consulted before the matrix, for every mode and category, so a grant can
// rescue a call the mode would otherwise deny or prompt for.
func TestSessionAlwaysOverridesMatrix(t *testing.T) {
	for _, mode := range Modes() {
		for _, cat := range allCategories {
			granted := map[string]struct{}{"granted_tool": {}}
			got, reason := Evaluate(mode, "granted_tool", cat, granted)
			if got != Allow {
				t.Errorf("grant ignored: Evaluate(%q, %q, granted) = %v (%s), want Allow", mode, cat, got, reason)
			}
		}
	}
}

// TestSessionAlwaysIsPerToolName covers the reason grants are not keyed by
// category: allowing one write tool must not allow a different one.
func TestSessionAlwaysIsPerToolName(t *testing.T) {
	granted := map[string]struct{}{"write_file": {}}
	if got, _ := Evaluate(ModeReadOnly, "write_file", tools.CategoryWrite, granted); got != Allow {
		t.Fatalf("granted tool = %v, want Allow", got)
	}
	if got, _ := Evaluate(ModeReadOnly, "delete_file", tools.CategoryWrite, granted); got != Ask {
		t.Fatalf("ungranted sibling in the same category = %v, want Ask", got)
	}
}

// TestPlanModeHasNoApprovalEscapeHatch is the defining property of plan mode:
// it denies rather than asks, so a user cannot confirm their way into an edit
// without leaving the mode.
func TestPlanModeHasNoApprovalEscapeHatch(t *testing.T) {
	for _, cat := range allCategories {
		got, _ := Evaluate(ModePlan, "any_tool", cat, nil)
		if got == Ask {
			t.Errorf("plan mode returned Ask for category %q; it must Deny or Allow", cat)
		}
	}
}

func TestNormalizeFallsBackToDefault(t *testing.T) {
	cases := []struct {
		in   string
		want Mode
	}{
		{"", DefaultMode},
		{"auto-edit", ModeAutoEdit},
		{"plan", ModePlan},
		{"read-only", ModeReadOnly},
		{"full-auto", ModeFullAuto},
		{"AUTO-EDIT", DefaultMode},
		{"nonsense", DefaultMode},
		{"readonly", DefaultMode},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEvaluateNormalizesUnknownMode keeps a corrupt or hand-edited session
// metadata value from failing open: an unknown mode behaves exactly like the
// configured default.
func TestEvaluateNormalizesUnknownMode(t *testing.T) {
	for _, cat := range allCategories {
		got, reason := Evaluate(Mode("corrupt"), "t", cat, nil)
		want, wantReason := Evaluate(DefaultMode, "t", cat, nil)
		if got != want || reason != wantReason {
			t.Errorf("unknown mode category %q = (%v, %q), want default's (%v, %q)", cat, got, reason, want, wantReason)
		}
	}
}

func TestNextCyclesTheRing(t *testing.T) {
	seen := make([]Mode, 0, len(Modes()))
	cur := ModePlan
	for range Modes() {
		seen = append(seen, cur)
		cur = Next(cur)
	}
	want := []Mode{ModePlan, ModeReadOnly, ModeAutoEdit, ModeFullAuto}
	if len(seen) != len(want) {
		t.Fatalf("ring length %d, want %d", len(seen), len(want))
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("ring[%d] = %q, want %q", i, seen[i], want[i])
		}
	}
	if cur != ModePlan {
		t.Fatalf("ring did not wrap: got %q, want %q", cur, ModePlan)
	}
}

// TestNextFromUnknownModeAdvancesFromDefault pins that an unrecognised current
// mode is normalized first, so Shift+Tab on a corrupt session lands on the
// default's successor instead of jumping to an arbitrary ring position.
func TestNextFromUnknownModeAdvancesFromDefault(t *testing.T) {
	want := Next(DefaultMode)
	if got := Next(Mode("nonsense")); got != want {
		t.Fatalf("Next(unknown) = %q, want the default's successor %q", got, want)
	}
}

func TestValidMode(t *testing.T) {
	for _, m := range Modes() {
		if !ValidMode(m) {
			t.Errorf("ValidMode(%q) = false for a ring member", m)
		}
	}
	for _, s := range []string{"", "PLAN", "full_auto", "chat"} {
		if ValidMode(Mode(s)) {
			t.Errorf("ValidMode(%q) = true for a non-member", s)
		}
	}
}

// TestEvaluateModeSwitch pins the switch_mode policy: entering plan is the
// only boundary the model may cross on its own, leaving plan is refused
// (that gate belongs to exit_plan, even with a session grant), every other
// transition is gated on a user decision, and an explicit per-session grant
// outranks the sideways rule exactly as it does for Evaluate.
func TestEvaluateModeSwitch(t *testing.T) {
	cases := []struct {
		name        string
		cur, target Mode
		grants      map[string]struct{}
		want        Verdict
	}{
		{"leaving plan is refused", ModePlan, ModeAutoEdit, nil, Deny},
		{"leaving plan to read-only is refused", ModePlan, ModeReadOnly, nil, Deny},
		{"leaving plan to full-auto is refused", ModePlan, ModeFullAuto, nil, Deny},
		{"grant does not skip leaving plan", ModePlan, ModeAutoEdit, map[string]struct{}{"switch_mode": {}}, Deny},
		{"entering plan is direct", ModeAutoEdit, ModePlan, nil, Allow},
		{"entering plan from read-only is direct", ModeReadOnly, ModePlan, nil, Allow},
		{"entering plan from full-auto is direct", ModeFullAuto, ModePlan, nil, Allow},
		{"staying in plan is a no-op", ModePlan, ModePlan, nil, Allow},
		{"sideways auto-edit to full-auto asks", ModeAutoEdit, ModeFullAuto, nil, Ask},
		{"sideways full-auto to auto-edit asks", ModeFullAuto, ModeAutoEdit, nil, Ask},
		{"sideways read-only to auto-edit asks", ModeReadOnly, ModeAutoEdit, nil, Ask},
		{"sideways auto-edit to read-only asks", ModeAutoEdit, ModeReadOnly, nil, Ask},
		{"grant rescues a sideways switch", ModeAutoEdit, ModeFullAuto, map[string]struct{}{"switch_mode": {}}, Allow},
		{"corrupt current mode normalizes to default", Mode("corrupt"), ModeAutoEdit, nil, Allow},
		{"corrupt target normalizes to default", ModeReadOnly, Mode("garbage"), nil, Ask},
	}
	for _, c := range cases {
		got, reason := EvaluateModeSwitch(c.cur, c.target, c.grants)
		if got != c.want {
			t.Errorf("%s: EvaluateModeSwitch(%q, %q) = %v (%s), want %v", c.name, c.cur, c.target, got, reason, c.want)
		}
		if c.want == Ask && reason == "" {
			t.Errorf("%s: Ask with an empty reason", c.name)
		}
	}
}

// TestEvaluateModeSwitchDeniesOnlyLeavingPlan pins that switch_mode cannot
// leave plan (that gate is exit_plan). Every other pair is Allow or Ask.
func TestEvaluateModeSwitchDeniesOnlyLeavingPlan(t *testing.T) {
	for _, cur := range Modes() {
		for _, target := range Modes() {
			got, _ := EvaluateModeSwitch(cur, target, nil)
			leaving := cur == ModePlan && target != ModePlan
			if leaving && got != Deny {
				t.Errorf("EvaluateModeSwitch(%q, %q) = %v, want Deny", cur, target, got)
			}
			if !leaving && got == Deny {
				t.Errorf("EvaluateModeSwitch(%q, %q) = Deny", cur, target)
			}
		}
	}
}
