package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type repoFiles struct {
	site, theme, flyout, kit, lamp, settingsWindow, settings, mockup, sketchFlyout string
}

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func repoSources(t *testing.T) repoFiles {
	t.Helper()
	return repoFiles{
		site:           readRepo(t, siteHTML),
		theme:          readRepo(t, themeGo),
		flyout:         readRepo(t, flyoutGo),
		kit:            readRepo(t, uiKitGo),
		lamp:           readRepo(t, lampGo),
		settingsWindow: readRepo(t, settingsWindowGo),
		settings:       readRepo(t, settingsCreateGo),
		mockup:         readRepo(t, mockupTemplate),
		sketchFlyout:   readRepo(t, flyoutTemplate),
	}
}

// sketchDrift runs the sketch half of the gate against the sources with one
// template replaced.
func sketchDrift(f repoFiles, mockup, sketchFlyout string) []string {
	sources := map[string]string{
		siteHTML: f.site, themeGo: f.theme, flyoutGo: f.flyout, uiKitGo: f.kit,
		lampGo: f.lamp, settingsWindowGo: f.settingsWindow, settingsCreateGo: f.settings,
		mockupTemplate: mockup, flyoutTemplate: sketchFlyout,
	}
	problems, _ := checkSketches(f.theme, mockup, sketchFlyout, sources)
	return problems
}

// TestRepositoryTokensMatch is the gate itself, run as a test so a parser
// change fails beside the parser.
func TestRepositoryTokensMatch(t *testing.T) {
	f := repoSources(t)
	problems, width := checkTokens(f.site, f.theme, f.flyout, f.kit)
	if len(problems) > 0 {
		t.Fatalf("the site has drifted from the app:\n%s", strings.Join(problems, "\n"))
	}
	if width != "420" {
		t.Fatalf("the panel width is %q, want 420", width)
	}
	if problems := sketchDrift(f, f.mockup, f.sketchFlyout); len(problems) > 0 {
		t.Fatalf("a sketch template has drifted from the app:\n%s", strings.Join(problems, "\n"))
	}
}

// TestSeededTokenDriftFails proves the parser reads the CSS rather than
// passing on anything. Each seed is one character, the size of a real drift.
func TestSeededTokenDriftFails(t *testing.T) {
	f := repoSources(t)
	seeded := strings.Replace(f.site, "--fl-accent:#4f46e5", "--fl-accent:#4f46e6", 1)
	if seeded == f.site {
		t.Fatal("the light accent seed did not apply; the CSS token block moved")
	}
	problems, _ := checkTokens(seeded, f.theme, f.flyout, f.kit)
	if !hasProblem(problems, "--fl-accent") {
		t.Fatalf("a drifted light accent was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestSeededMetricDriftFails(t *testing.T) {
	f := repoSources(t)
	seeded := strings.Replace(f.site, ".fly-head{height:46px;", ".fly-head{height:47px;", 1)
	if seeded == f.site {
		t.Fatal("the header height seed did not apply; the flyout CSS moved")
	}
	problems, _ := checkTokens(seeded, f.theme, f.flyout, f.kit)
	if !hasProblem(problems, ".fly-head{height:46px") {
		t.Fatalf("a drifted header height was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

// TestSeededDeclarationDriftFails changes the header's Height(46) while the
// PIN and password inputs keep their own Height(46), the duplicate that let a
// bare-literal check pass after the declaration moved.
func TestSeededDeclarationDriftFails(t *testing.T) {
	f := repoSources(t)
	const decl = `ui.Row(c).Height(46).Padding(0, 10, 0, 14).Gap(8)`
	seeded := strings.Replace(f.flyout, decl, `ui.Row(c).Height(47).Padding(0, 10, 0, 14).Gap(8)`, 1)
	if seeded == f.flyout {
		t.Fatal("the header declaration seed did not apply; the flyout header moved")
	}
	if strings.Count(seeded, "Height(46)") == 0 {
		t.Fatal("the seed left no duplicate Height(46); it cannot prove the check pins the declaration")
	}
	problems, _ := checkTokens(f.site, f.theme, seeded, f.kit)
	if !hasProblem(problems, "the header height") {
		t.Fatalf("a moved header declaration was not reported while a duplicate remained:\n%s", strings.Join(problems, "\n"))
	}
}

func TestSeededWidthDriftFails(t *testing.T) {
	f := repoSources(t)
	seeded := strings.Replace(f.site, ".fly-wrap{justify-self:center;width:420px", ".fly-wrap{justify-self:center;width:380px", 1)
	if seeded == f.site {
		t.Fatal("the panel width seed did not apply; the .fly-wrap rule moved")
	}
	problems, _ := checkTokens(seeded, f.theme, f.flyout, f.kit)
	if !hasProblem(problems, "the page lays the panel out") {
		t.Fatalf("a drifted panel width was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestMockupTemplateTokenDriftFails(t *testing.T) {
	f := repoSources(t)
	seeded := strings.Replace(f.mockup, "--accent: #4f46e5", "--accent: #4f46e6", 1)
	if seeded == f.mockup {
		t.Fatal("the settings accent seed did not apply; the palette block moved")
	}
	problems := sketchDrift(f, seeded, f.sketchFlyout)
	if !hasProblem(problems, "--accent") || !hasProblem(problems, "mockup-template.html") {
		t.Fatalf("a drifted settings accent was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestMockupTemplateMetricDriftFails(t *testing.T) {
	f := repoSources(t)
	seeded := strings.Replace(f.mockup, ".page-title { font-size: 18px; font-weight: 650; }", ".page-title { font-size: 19px; font-weight: 650; }", 1)
	if seeded == f.mockup {
		t.Fatal("the page title seed did not apply; the .page-title rule moved")
	}
	problems := sketchDrift(f, seeded, f.sketchFlyout)
	if !hasProblem(problems, ".page-title { font-size: 18px") {
		t.Fatalf("a drifted page title size was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestFlyoutTemplateTokenDriftFails(t *testing.T) {
	f := repoSources(t)
	seeded := strings.Replace(f.sketchFlyout, "--warning: #b76a12", "--warning: #b76a13", 1)
	if seeded == f.sketchFlyout {
		t.Fatal("the warning seed did not apply; the palette block moved")
	}
	problems := sketchDrift(f, f.mockup, seeded)
	if !hasProblem(problems, "--warning") || !hasProblem(problems, "flyout-template.html") {
		t.Fatalf("a drifted warning token was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestFlyoutTemplateMetricDriftFails(t *testing.T) {
	f := repoSources(t)
	seeded := strings.Replace(f.sketchFlyout, "height: 46px; flex: none;", "height: 47px; flex: none;", 1)
	if seeded == f.sketchFlyout {
		t.Fatal("the header height seed did not apply; the .flyout-head rule moved")
	}
	problems := sketchDrift(f, f.mockup, seeded)
	if !hasProblem(problems, "height: 46px; flex: none;") {
		t.Fatalf("a drifted flyout header height was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestFlyoutHeightRuleDriftFails(t *testing.T) {
	f := repoSources(t)
	seeded := strings.Replace(f.sketchFlyout, "FLYOUT_BASE = 439", "FLYOUT_BASE = 438", 1)
	if seeded == f.sketchFlyout {
		t.Fatal("the base height seed did not apply; the height rule moved")
	}
	problems := sketchDrift(f, f.mockup, seeded)
	if !hasProblem(problems, "FLYOUT_BASE") {
		t.Fatalf("a drifted base height was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestSketchWindowSizeDriftFails(t *testing.T) {
	f := repoSources(t)
	flyoutSeed := strings.Replace(f.sketchFlyout, "width: 420px; height: var(--fh, 500px);", "width: 419px; height: var(--fh, 500px);", 1)
	if flyoutSeed == f.sketchFlyout {
		t.Fatal("the flyout width seed did not apply")
	}
	if problems := sketchDrift(f, f.mockup, flyoutSeed); !hasProblem(problems, "the flyout width") {
		t.Fatalf("a drifted flyout width was not reported:\n%s", strings.Join(problems, "\n"))
	}
	mockupSeed := strings.Replace(f.mockup, "width: 720px; height: 500px;", "width: 721px; height: 500px;", 1)
	if mockupSeed == f.mockup {
		t.Fatal("the settings width seed did not apply")
	}
	if problems := sketchDrift(f, mockupSeed, f.sketchFlyout); !hasProblem(problems, "the settings width") {
		t.Fatalf("a drifted settings width was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestPalettesReadsBothAppearances(t *testing.T) {
	theme := `func paletteFor(dark bool) palette {
	if dark {
		return palette{
			panel:       ui.Hex("#111111"),
			accent:      ui.Hex("#222222"),
		}
	}
	return palette{
		panel:       ui.Hex("#eeeeee"),
		accent:      ui.Hex("#333333"),
	}
}`
	dark, light := palettes(theme)
	if dark["panel"] != "#111111" || dark["accent"] != "#222222" {
		t.Fatalf("dark = %v", dark)
	}
	if light["panel"] != "#eeeeee" || light["accent"] != "#333333" {
		t.Fatalf("light = %v", light)
	}
}

func TestSitePalettesReadsBothBlocks(t *testing.T) {
	site := `.fly-wrap,#bmMedia{
  --fl-panel:#f2f2f5; --fl-accent:#4f46e5;
}
html[data-theme="dark"] .fly-wrap,html[data-theme="dark"] #bmMedia{
  --fl-panel:#1b1c20; --fl-accent:#a5b4fc;
}`
	light, dark := sitePalettes(site)
	if light["panel"] != "#f2f2f5" || light["accent"] != "#4f46e5" {
		t.Fatalf("light = %v", light)
	}
	if dark["panel"] != "#1b1c20" || dark["accent"] != "#a5b4fc" {
		t.Fatalf("dark = %v", dark)
	}
}

// TestTemplatePalettesIgnoresTheChromeBodyRule proves the parser starts at
// the palette rule, not the first rule that happens to mention body.
func TestTemplatePalettesIgnoresTheChromeBodyRule(t *testing.T) {
	src := `html, body { min-height: 100%; }
  body {
    --panel: #f2f2f5; --accent-soft: #eeedfd;
  }
  body.dark {
    --panel: #1b1c20; --accent-soft: #2b2c47;
  }
  body.dark .toolbar { background: #101013; }`
	light, dark, ok := templatePalettes(src)
	if !ok {
		t.Fatal("the palette rules were not found")
	}
	if light["panel"] != "#f2f2f5" || light["accent-soft"] != "#eeedfd" {
		t.Fatalf("light = %v", light)
	}
	if dark["panel"] != "#1b1c20" || dark["accent-soft"] != "#2b2c47" {
		t.Fatalf("dark = %v", dark)
	}
}

func TestFlyWrapWidthReadsTheLayoutDeclaration(t *testing.T) {
	site := ".fly-wrap{justify-self:center;width:420px;max-width:none;zoom:var(--fly-zoom,.77)}"
	if got, ok := flyWrapWidth(site); !ok || got != "420" {
		t.Fatalf("flyWrapWidth = %q, %v", got, ok)
	}
	if _, ok := flyWrapWidth(".fly-wrap{width:100%}"); ok {
		t.Fatal("a percentage width was read as a pixel width")
	}
}

func hasProblem(problems []string, want string) bool {
	for _, p := range problems {
		if strings.Contains(p, want) {
			return true
		}
	}
	return false
}
