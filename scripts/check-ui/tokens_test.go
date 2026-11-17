package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func repoSources(t *testing.T) (site, theme, flyout, kit string) {
	t.Helper()
	return readRepo(t, siteHTML), readRepo(t, themeGo), readRepo(t, flyoutGo), readRepo(t, uiKitGo)
}

// TestRepositoryTokensMatch is the gate itself, run as a test so a parser
// change fails beside the parser.
func TestRepositoryTokensMatch(t *testing.T) {
	site, theme, flyout, kit := repoSources(t)
	problems, width := checkTokens(site, theme, flyout, kit)
	if len(problems) > 0 {
		t.Fatalf("the site has drifted from the app:\n%s", strings.Join(problems, "\n"))
	}
	if width != "420" {
		t.Fatalf("the panel width is %q, want 420", width)
	}
}

// TestSeededTokenDriftFails proves the parser reads the CSS rather than
// passing on anything. Each seed is one character, the size of a real drift.
func TestSeededTokenDriftFails(t *testing.T) {
	site, theme, flyout, kit := repoSources(t)
	seeded := strings.Replace(site, "--fl-accent:#4f46e5", "--fl-accent:#4f46e6", 1)
	if seeded == site {
		t.Fatal("the light accent seed did not apply; the CSS token block moved")
	}
	problems, _ := checkTokens(seeded, theme, flyout, kit)
	if !hasProblem(problems, "--fl-accent") {
		t.Fatalf("a drifted light accent was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestSeededMetricDriftFails(t *testing.T) {
	site, theme, flyout, kit := repoSources(t)
	seeded := strings.Replace(site, ".fly-head{height:46px;", ".fly-head{height:47px;", 1)
	if seeded == site {
		t.Fatal("the header height seed did not apply; the flyout CSS moved")
	}
	problems, _ := checkTokens(seeded, theme, flyout, kit)
	if !hasProblem(problems, ".fly-head{height:46px") {
		t.Fatalf("a drifted header height was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

// TestSeededDeclarationDriftFails changes the header's Height(46) while the
// PIN and password inputs keep their own Height(46), the duplicate that let a
// bare-literal check pass after the declaration moved.
func TestSeededDeclarationDriftFails(t *testing.T) {
	site, theme, flyout, kit := repoSources(t)
	const decl = `ui.Row(c).Height(46).Padding(0, 10, 0, 14).Gap(8)`
	seeded := strings.Replace(flyout, decl, `ui.Row(c).Height(47).Padding(0, 10, 0, 14).Gap(8)`, 1)
	if seeded == flyout {
		t.Fatal("the header declaration seed did not apply; the flyout header moved")
	}
	if strings.Count(seeded, "Height(46)") == 0 {
		t.Fatal("the seed left no duplicate Height(46); it cannot prove the check pins the declaration")
	}
	problems, _ := checkTokens(site, theme, seeded, kit)
	if !hasProblem(problems, "the header height") {
		t.Fatalf("a moved header declaration was not reported while a duplicate remained:\n%s", strings.Join(problems, "\n"))
	}
}

func TestSeededWidthDriftFails(t *testing.T) {
	site, theme, flyout, kit := repoSources(t)
	seeded := strings.Replace(site, ".fly-wrap{justify-self:center;width:420px", ".fly-wrap{justify-self:center;width:380px", 1)
	if seeded == site {
		t.Fatal("the panel width seed did not apply; the .fly-wrap rule moved")
	}
	problems, _ := checkTokens(seeded, theme, flyout, kit)
	if !hasProblem(problems, "the page lays the panel out") {
		t.Fatalf("a drifted panel width was not reported:\n%s", strings.Join(problems, "\n"))
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
