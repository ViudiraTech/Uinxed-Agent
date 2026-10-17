package tui

import "testing"

// A screen is only worth a timer while something on it is frame-driven. The
// failure this pins: the effort slider used to arm the ticker just by being open,
// so an idle session with supercode off kept repainting the panel eight times a
// second for an effect it was not showing.
func TestNeedsAnimationCoversTheFrameDrivenSurfaces(t *testing.T) {
	m := layoutModel(t)
	if m.needsAnimation() {
		t.Fatal("an idle session with supercode off animates nothing")
	}

	m.overlay = overlayEffort
	if m.needsAnimation() {
		t.Fatal("an idle slider with supercode off animates nothing")
	}

	m.overlay = overlayNone
	m.session.Metadata = map[string]any{"supercode": true}
	if m.needsAnimation() {
		t.Fatal("the composer ring is static; supercode alone animates nothing")
	}

	m.overlay = overlayEffort
	if !m.needsAnimation() {
		t.Fatal("the surface needs frames while its panel is open")
	}
	m.overlay = overlayNone

	m.session.Metadata = map[string]any{"supercode": false}
	m.busy = true
	if !m.needsAnimation() {
		t.Fatal("a running turn is frame-driven")
	}
}

// Two live chains would advance the frame twice per interval and run every effect
// at double speed, so arming has to be idempotent.
func TestStartAnimationArmsOneChain(t *testing.T) {
	m := layoutModel(t)
	m.cfg.Animations = true
	m.session.Metadata = map[string]any{"supercode": true}
	m.overlay = overlayEffort

	if cmd := m.startAnimation(); cmd == nil {
		t.Fatal("supercode on did not arm the ticker")
	}
	if !m.ticking {
		t.Fatal("arming the ticker did not mark it in flight")
	}
	if cmd := m.startAnimation(); cmd != nil {
		t.Fatal("a second tick chain was armed")
	}
}

func TestStartAnimationRespectsTheToggle(t *testing.T) {
	m := layoutModel(t)
	m.cfg.Animations = false
	m.session.Metadata = map[string]any{"supercode": true}
	m.overlay = overlayEffort

	if cmd := m.startAnimation(); cmd != nil {
		t.Fatal("animations are disabled; no timer should be armed")
	}
	if m.ticking {
		t.Fatal("ticking must stay false when animations are off")
	}
}

// The tick has to advance the frame and re-arm while supercode is on, which is
// what makes the panel field and the composer ripple move on an idle session.
func TestTickChainRunsInSupercodeMode(t *testing.T) {
	m := layoutModel(t)
	m.cfg.Animations = true
	m.session.Metadata = map[string]any{"supercode": true}
	m.overlay = overlayEffort
	m.ticking = true

	before := m.activityFrame
	if _, cmd := m.Update(animationTickMsg{}); cmd == nil {
		t.Fatal("the chain stopped while supercode is on")
	}
	if m.activityFrame != before+1 {
		t.Fatalf("frame went %d -> %d, want one step", before, m.activityFrame)
	}
}

// Closing the overlay and leaving supercode off must let the chain die, or an
// idle session keeps repainting at 8 fps forever.
func TestTickChainStopsWhenNothingMoves(t *testing.T) {
	m := layoutModel(t)
	m.cfg.Animations = true
	m.ticking = true

	before := m.activityFrame
	if _, cmd := m.Update(animationTickMsg{}); cmd != nil {
		t.Fatal("an idle screen re-armed the ticker")
	}
	if m.activityFrame != before {
		t.Fatal("an idle screen advanced a frame")
	}
	if m.ticking {
		t.Fatal("ticking stayed true after the chain stopped")
	}
}
