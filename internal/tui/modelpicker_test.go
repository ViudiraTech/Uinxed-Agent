package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/app"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func newPickerModel(t *testing.T, models []string) *Model {
	t.Helper()
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertProvider(config.Provider{ID: "local", Name: "Local", BaseURL: "http://127.0.0.1:1/v1", Models: models}, "k"); err != nil {
		t.Fatal(err)
	}
	ctrl, err := app.New(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctrl.Close() })
	return &Model{ctrl: ctrl, session: domain.Session{ID: "s", ProviderID: "local"}}
}

// The picker must be populated from the provider's configured models without
// waiting for GET /models; blocking here is what made /model look hung.
func TestFetchModelsOpensPickerBeforeDiscoveryCompletes(t *testing.T) {
	m := newPickerModel(t, []string{"model-a", "model-b"})
	cmd := m.fetchModels()
	if m.overlay != overlayPicker || m.pickerPurpose != "model" {
		t.Fatalf("overlay=%v purpose=%q", m.overlay, m.pickerPurpose)
	}
	if len(m.picker.Items) != 2 || m.picker.Items[0].ID != "model-a" || m.picker.Items[1].ID != "model-b" {
		t.Fatalf("items=%#v", m.picker.Items)
	}
	if !strings.Contains(m.picker.Title, "loading") {
		t.Fatalf("title=%q", m.picker.Title)
	}
	if cmd == nil {
		t.Fatal("expected a discovery command")
	}
}

// A discovery failure must leave the configured models selectable rather than
// replacing the list with an error row.
func TestFailedDiscoveryKeepsConfiguredModels(t *testing.T) {
	m := newPickerModel(t, []string{"model-a", "model-b"})
	m.fetchModels()
	models, err := m.ctrl.Models(context.Background(), "local")
	if err == nil {
		t.Fatal("expected the unreachable endpoint to fail discovery")
	}
	_, _ = m.Update(modelsMsg{models: models, err: err})
	if len(m.picker.Items) != 2 {
		t.Fatalf("items=%#v", m.picker.Items)
	}
	if m.picker.Items[0].ID != "model-a" {
		t.Fatalf("items=%#v", m.picker.Items)
	}
	if !strings.Contains(m.picker.Title, "configured list") {
		t.Fatalf("title=%q", m.picker.Title)
	}
	if m.errorText == "" {
		t.Fatal("expected the failure to be surfaced")
	}
}

func TestModelPickerKeepsQueryAcrossDiscoveryRefresh(t *testing.T) {
	m := &Model{}
	m.showModelPicker("Model · loading…", modelPickerItems([]string{"a", "b", "c"}))
	m.picker.SetQuery("c")
	m.showModelPicker("Model", modelPickerItems([]string{"a", "b", "c", "d"}))
	if m.picker.Query != "c" {
		t.Fatalf("query=%q want %q", m.picker.Query, "c")
	}
	if len(m.picker.Filtered) != 1 {
		t.Fatalf("filtered=%v", m.picker.Filtered)
	}
}
