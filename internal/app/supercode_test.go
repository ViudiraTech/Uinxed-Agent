package app

import (
	"context"
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
)

// The runtime reads supercode from session metadata, not from the config store,
// so a session that is born without the key silently ignores a config default of
// true while the UI — which does fall back to config — still shows the mode as on.
// Copying the default into the new session is what keeps the two in agreement.
func TestNewSessionCarriesSupercodeDefault(t *testing.T) {
	ctrl := titleController(t, "")
	if err := ctrl.Config.Update(func(c *config.Config) error { c.Supercode = true; return nil }); err != nil {
		t.Fatal(err)
	}
	s, err := ctrl.NewSession(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	if on, _ := s.Metadata["supercode"].(bool); !on {
		t.Fatalf("new session metadata lost the supercode default: %v", s.Metadata)
	}

	loaded, err := ctrl.Store.LoadSession(context.Background(), s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if on, _ := loaded.Metadata["supercode"].(bool); !on {
		t.Fatalf("stored session lost the supercode default: %v", loaded.Metadata)
	}
}
