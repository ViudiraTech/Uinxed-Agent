package tui

type Focus int

const (
	FocusPrompt Focus = iota
	FocusChat
	FocusSidebar
	FocusModal
	FocusCommandPalette
	FocusDiff
	FocusPicker
	FocusTodos
)

// FocusOverlay remains the generic modal focus for informational/confirm dialogs.
const FocusOverlay = FocusModal

type FocusManager struct {
	current  Focus
	previous Focus
}

func (f *FocusManager) Set(v Focus) {
	if f.current != v {
		f.previous = f.current
		f.current = v
	}
}
func (f *FocusManager) Restore()      { f.current, f.previous = f.previous, f.current }
func (f FocusManager) Current() Focus { return f.current }
