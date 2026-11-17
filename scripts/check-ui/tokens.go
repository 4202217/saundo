// check-ui tokens keeps the landing page's hand-built flyout honest about its
// numbers.
//
// site/index.html draws the tray panel again in HTML and CSS. It copies the
// app's colour tokens, the DIPs of its rows and type, and its width, and
// nothing links the two, so a change on either side can leave the other
// behind without a red build. The words are the words subcommand's job,
// because they need the rendered view rather than a text file.
//
// What it cannot see is shape: a missing affordance, a fill in the wrong
// colour, a control that grew heavier than the real one.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	siteHTML = "site/index.html"
	themeGo  = "internal/app/theme.go"
	flyoutGo = "internal/app/flyout.go"
	uiKitGo  = "internal/app/ui_kit.go"
)

// tokens are the palette fields the site re-declares as --fl-* variables. The
// app owns the value; the site must copy it in both appearances.
var tokens = []string{
	"panel", "card", "cardHover", "pill", "pillHover",
	"line", "line2", "fg", "fg2", "muted", "faint",
	"accent", "sel", "green", "ctl",
}

// metric ties one declaration in the view code to the rule it produced in the
// site's CSS. goContext is the declaration with enough around it to be unique:
// a bare Height(46) also appears in the PIN and password inputs, so a header
// change would keep passing on their duplicate. Both sides must still hold:
// move one without the other and this fails.
type metric struct{ name, goFile, goContext, cssAnchor string }

var metrics = []metric{
	{"the header height", flyoutGo, `ui.Row(c).Height(46).Padding(0, 10, 0, 14).Gap(8)`, ".fly-head{height:46px"},
	{"the header padding", flyoutGo, `ui.Row(c).Height(46).Padding(0, 10, 0, 14).Gap(8)`, ".fly-head{height:46px;flex:none;display:flex;align-items:center;gap:8px;padding:0 10px 0 14px"},
	{"the app mark", flyoutGo, `ui.Icon(c, iconMark).Size(21, 21)`, ".fly-mark{width:21px;height:21px"},
	{"an icon button", uiKitGo, `ui.ButtonBase(c).Size(26, 26)`, ".fly-ibtn{width:26px;height:26px"},
	{"a receiver row", flyoutGo, `ui.Row(c).Height(56).Padding(9, 12).Gap(10)`, ".fdev{height:56px"},
	{"the receiver padding", flyoutGo, `ui.Row(c).Height(56).Padding(9, 12).Gap(10)`, ".fdev{height:56px;display:flex;align-items:center;gap:10px;padding:9px 12px"},
	{"the app name size", flyoutGo, `ui.Text(c, locales.AppName()).FontSize(13)`, ".fly-app{font-size:13px"},
	{"the subtitle size", flyoutGo, `ui.Text(c, locales.AppSubtitle()).FontSize(8.5)`, ".fly-sub{font-size:8.5px"},
	{"the subtitle tracking", flyoutGo, `ui.Text(c, locales.AppSubtitle()).FontSize(8.5).FontWeight(600).LetterSpacing(1.1)`, ".fly-sub{font-size:8.5px;font-weight:600;letter-spacing:1.1px"},
	{"the section label size", uiKitGo, `ui.Text(c, label).FontSize(11.5).FontWeight(600).LetterSpacing(0.55)`, ".fly-slabel{font-size:11.5px"},
	{"the section label tracking", uiKitGo, `ui.Text(c, label).FontSize(11.5).FontWeight(600).LetterSpacing(0.55)`, "letter-spacing:.55px"},
}

var (
	goPair   = regexp.MustCompile(`([A-Za-z0-9]+)\s*:\s*ui\.Hex\("#([0-9a-fA-F]{6})"\)`)
	cssToken = regexp.MustCompile(`--fl-([A-Za-z0-9]+):\s*(#[0-9a-fA-F]{3,8})`)
)

// runTokens reads the four sources and reports every way the site's copy has
// parted from the app's own values.
func runTokens(root string) error {
	read := func(rel string) (string, error) {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	site, err := read(siteHTML)
	if err != nil {
		return err
	}
	theme, err := read(themeGo)
	if err != nil {
		return err
	}
	flyout, err := read(flyoutGo)
	if err != nil {
		return err
	}
	kit, err := read(uiKitGo)
	if err != nil {
		return err
	}
	problems, width := checkTokens(site, theme, flyout, kit)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Println("check-ui tokens:", p)
		}
		return fmt.Errorf("tokens: %d drift(s)", len(problems))
	}
	fmt.Printf("check-ui: %d tokens in two appearances, %d metrics, the panel at %s px: the replica still follows the app\n",
		len(tokens), len(metrics), width)
	return nil
}

// checkTokens compares the site with the Go sources and returns every drift,
// plus the width it read, for the pass line.
func checkTokens(site, theme, flyout, kit string) (problems []string, width string) {
	appDark, appLight := palettes(theme)
	siteLight, siteDark := sitePalettes(site)
	for _, name := range tokens {
		for _, appearance := range []struct {
			label    string
			app, web map[string]string
		}{
			{"dark", appDark, siteDark},
			{"light", appLight, siteLight},
		} {
			a, ok := appearance.app[name]
			if !ok {
				problems = append(problems, fmt.Sprintf(
					"%s: internal/app/theme.go no longer declares %q, so the site's --fl-%s answers to nothing", appearance.label, name, name))
				continue
			}
			w, ok := appearance.web[name]
			if !ok {
				problems = append(problems, fmt.Sprintf(
					"%s: site/index.html declares no --fl-%s for the app's %s", appearance.label, name, a))
				continue
			}
			if !strings.EqualFold(a, w) {
				problems = append(problems, fmt.Sprintf(
					"%s: --fl-%s is %s on the site and %s in theme.go", appearance.label, name, w, a))
			}
		}
	}

	sources := map[string]string{flyoutGo: flyout, uiKitGo: kit}
	for _, m := range metrics {
		if !strings.Contains(sources[m.goFile], m.goContext) {
			problems = append(problems, fmt.Sprintf(
				"%s no longer declares %s where the CSS copies it; update site/index.html and this check", m.goFile, m.name))
		}
		if !strings.Contains(site, m.cssAnchor) {
			problems = append(problems, fmt.Sprintf(
				"site/index.html no longer carries %q; the view still declares %s", m.cssAnchor, m.name))
		}
	}

	// The page draws the panel at the app's own width and scales it to fit, so
	// there is one layout to keep faithful rather than one per breakpoint.
	if appWidth, ok := digitsIn(flyout, `flyoutWidth\s*=\s*(\d+)`); !ok {
		problems = append(problems, "internal/app/flyout.go no longer declares flyoutWidth")
	} else if webWidth, ok := flyWrapWidth(site); !ok {
		problems = append(problems, "site/index.html no longer gives .fly-wrap a pixel width")
	} else if appWidth != webWidth {
		problems = append(problems, fmt.Sprintf(
			"the page lays the panel out at %s px and flyoutWidth is %s", webWidth, appWidth))
	} else {
		width = webWidth
	}
	return problems, width
}

// palettes splits theme.go at its two palette literals: the one inside the
// dark branch first, the light one after it.
func palettes(src string) (dark, light map[string]string) {
	parts := strings.Split(src, "return palette{")
	if len(parts) < 3 {
		return map[string]string{}, map[string]string{}
	}
	return pairs(parts[1]), pairs(strings.Join(parts[2:], ""))
}

func pairs(block string) map[string]string {
	out := map[string]string{}
	for _, m := range goPair.FindAllStringSubmatch(block, -1) {
		out[m[1]] = "#" + m[2]
	}
	return out
}

// sitePalettes reads the light block and the dark override that follow the
// site's "real flyout tokens" comment.
func sitePalettes(src string) (light, dark map[string]string) {
	light = cssPairs(rule(src, ".fly-wrap,#bmMedia{"))
	dark = cssPairs(rule(src, `html[data-theme="dark"] .fly-wrap,html[data-theme="dark"] #bmMedia{`))
	return light, dark
}

func rule(src, selector string) string {
	i := strings.Index(src, selector)
	if i < 0 {
		return ""
	}
	rest := src[i+len(selector):]
	if end := strings.Index(rest, "}"); end >= 0 {
		return rest[:end]
	}
	return rest
}

func cssPairs(block string) map[string]string {
	out := map[string]string{}
	for _, m := range cssToken.FindAllStringSubmatch(block, -1) {
		out[m[1]] = m[2]
	}
	return out
}

// flyWrapWidth reads the pixel width the page lays the panel out at.
func flyWrapWidth(site string) (string, bool) {
	m := regexp.MustCompile(`(?m)^\.fly-wrap\{([^}]*)\}`).FindStringSubmatch(site)
	if m == nil {
		return "", false
	}
	return digitsIn(m[1], `width:(\d+)px`)
}

func digitsIn(src, expr string) (string, bool) {
	m := regexp.MustCompile(expr).FindStringSubmatch(src)
	if m == nil {
		return "", false
	}
	return m[1], true
}
