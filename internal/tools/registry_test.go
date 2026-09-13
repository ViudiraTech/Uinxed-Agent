package tools

import "testing"

func TestDefaultRegistryDoesNotExposeCurrentTimeTool(t *testing.T) {
	r := DefaultRegistry()
	if _, ok := r.Get("get_current_time"); ok {
		t.Fatal("get_current_time must not be exposed; runtime date/time is injected through the system prompt")
	}
}

// TestDefaultRegistryExposesPlanningTools pins the two tools the plan-mode
// workflow depends on: they must be registered so the runtime can resolve
// their category for the approval policy.
func TestDefaultRegistryExposesPlanningTools(t *testing.T) {
	r := DefaultRegistry()
	for _, name := range []string{"plan_write", "switch_mode", "exit_plan"} {
		if _, ok := r.Get(name); !ok {
			t.Fatalf("%s must be registered", name)
		}
		if _, ok := r.CategoryOf(name); !ok {
			t.Fatalf("%s must report a category", name)
		}
	}
}
