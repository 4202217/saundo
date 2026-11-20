//go:build preview

package app

// The CPU matrix driver: it renders the catalog through ui.Tester and writes
// one PNG per stem. check-ui render sets SAUNDO_PREVIEW_DIR and runs it.

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestRenderPreviews writes the catalog's states for a visual check, in both
// languages. Run with SAUNDO_PREVIEW_DIR set; it is not part of the regular
// suite.
func TestRenderPreviews(t *testing.T) {
	dir := os.Getenv("SAUNDO_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set SAUNDO_PREVIEW_DIR to write preview images")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A local build stamps git describe, the widest version the About pill
	// has to hold.
	oldVersion := Version
	Version = "0.4.2-5-g1e8e94c-dirty"
	defer func() { Version = oldVersion }()
	wanted := previewWanted()
	for _, tc := range []struct{ locale, suffix string }{
		{"en", "en"},
		{"zh-CN", "zh"},
	} {
		// The English README uses the en images and the Chinese README the zh
		// ones, so each language gets its own set.
		testLocale = tc.locale
		for _, sc := range Catalog() {
			renderScenario(t, dir, tc.locale, tc.suffix, sc, wanted)
		}
	}
	testLocale = "zh-CN"
}

// previewWanted filters the matrix and the platform driver by
// SAUNDO_PREVIEW_ONLY, a comma-separated list of name prefixes such as
// settings-audio,flyout, so a UI change renders only the pages it touched. An
// empty or unset value renders everything.
func previewWanted() func(string) bool {
	only := strings.TrimSpace(os.Getenv("SAUNDO_PREVIEW_ONLY"))
	if only == "" {
		return func(string) bool { return true }
	}
	var prefixes []string
	for _, p := range strings.Split(only, ",") {
		if p = strings.TrimSpace(p); p != "" {
			prefixes = append(prefixes, p)
		}
	}
	return func(name string) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
		return false
	}
}

// renderScenario builds a fresh fixture, applies the scenario, and writes the
// images the scenario declares.
func renderScenario(t *testing.T, dir, locale, suffix string, sc Scenario, wanted func(string) bool) {
	t.Helper()
	if !sc.anyWanted(wanted) {
		return
	}
	a := Fixture(locale)
	if sc.usesSettingsPage() {
		a.page = sc.Page
	}
	sc.Apply(a)
	view := sc.view(a)
	if view == nil {
		t.Fatalf("scenario %s: surface %d has no view", sc.ID, sc.Surface)
	}
	w, h := sc.Size(a)
	if sc.Frames > 0 {
		renderFrames(t, dir, locale, suffix, sc, view, w, h)
		return
	}
	for _, v := range sc.variants() {
		if !wanted(v.name) {
			continue
		}
		renderStill(t, dir, locale, suffix, sc, view, w, h, v.name, v.dark)
	}
}

// renderStill writes one settled frame. Scale grows the window the way the
// desktop's text-size setting does, Scroll shows the page under the pointer,
// and Move runs the hover probe between the settle frame and the last frame.
func renderStill(t *testing.T, dir, locale, suffix string, sc Scenario, view func(*ui.Context), w, h int, name string, dark bool) {
	t.Helper()
	var tt *ui.Tester
	if sc.Scale > 1 {
		ww, hh := scaledSize(w, h, sc.Scale)
		tt = ui.NewTesterWith(view, ww, hh, ui.TesterOptions{Preferences: ui.Preferences{TextScale: sc.Scale}})
	} else {
		tt = ui.NewTester(view, w, h)
	}
	tt.SetScale(2)
	tt.SetLocale(locale)
	tt.SetDark(dark)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	switch {
	case sc.Scroll != 0:
		// The pointer over the page makes the scrollbar and the fixed title
		// divider visible.
		tt.Move(600, 300)
		tt.Scroll(600, 300, 0, sc.Scroll)
		tt.Frame()
	case sc.Move != nil:
		sc.Move(tt)
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
	}
	writePNG(t, filepath.Join(dir, name+"-"+suffix+".png"), tt.Image())
}

// renderFrames writes the first frame and two more as the motion runs, to
// check an enter animation instead of its end state.
func renderFrames(t *testing.T, dir, locale, suffix string, sc Scenario, view func(*ui.Context), w, h int) {
	t.Helper()
	tt := ui.NewTester(view, w, h)
	tt.SetScale(2)
	tt.SetLocale(locale)
	for i, step := range []time.Duration{0, 70 * time.Millisecond, 250 * time.Millisecond} {
		if step > 0 {
			tt.Advance(step)
		}
		writePNG(t, filepath.Join(dir, fmt.Sprintf("%s-%s-%d.png", sc.Name, suffix, i)), tt.Image())
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
