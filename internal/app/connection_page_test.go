package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

// TestConnectionStabilityLabelKeepsItsWidth pins the stat cell layout: the
// label never truncates; the 60-second note carries the ellipsis. The en
// note is long enough that the row once squeezed the label to "Stabil…".
func TestConnectionStabilityLabelKeepsItsWidth(t *testing.T) {
	withLocale(t, "en")
	label := locales.SettingsHealthDrops()

	// The same text unconstrained: the width the label must keep. The row
	// keeps the text at its intrinsic width instead of the window's.
	ref := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Children(func() {
			ui.Text(c, label).FontSize(11.5)
		})
	}, 300, 60)
	want, ok := ref.Find(label)
	if !ok {
		t.Fatalf("the reference render is missing %q", label)
	}

	a := previewApp()
	a.page = pageConnection
	tt := ui.NewTester(a.settingsView, 720, 500)
	got, ok := tt.Find(label)
	if !ok {
		t.Fatalf("the connection page is missing %q; texts: %q", label, tt.Texts())
	}
	if got.W < want.W-0.5 {
		t.Fatalf("the stability label laid out at %.1f DIPs, want its full %.1f DIPs; the note must carry the truncation", got.W, want.W)
	}
}
