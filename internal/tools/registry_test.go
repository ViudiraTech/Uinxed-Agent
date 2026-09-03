package tools

import "testing"

func TestDefaultRegistryDoesNotExposeCurrentTimeTool(t *testing.T) {
	r := DefaultRegistry()
	if _, ok := r.Get("get_current_time"); ok {
		t.Fatal("get_current_time must not be exposed; runtime date/time is injected through the system prompt")
	}
}
