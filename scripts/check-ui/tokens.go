// check-ui tokens keeps the landing page's hand-built flyout and the two
// sketch templates honest about their numbers.
//
// site/index.html draws the tray panel again in HTML and CSS. It copies the
// app's colour tokens, the DIPs of its rows and type, and its width, and
// nothing links the two, so a change on either side can leave the other
// behind without a red build. The sketch templates under
// .claude/skills/ui-mockup/assets copy the same numbers. The words are the
// words subcommand's job, because they need the rendered view rather than a
// text file.
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
	siteHTML         = "site/index.html"
	themeGo          = "internal/app/theme.go"
	flyoutGo         = "internal/app/flyout.go"
	flyoutHeightGo   = "internal/app/flyout_height.go"
	uiKitGo          = "internal/app/ui_kit.go"
	lampGo           = "internal/app/lamp.go"
	settingsWindowGo = "internal/app/settings_window.go"
	connectionPageGo = "internal/app/connection_page.go"
	settingsCreateGo = "internal/app/settings_window_create.go"
	mockupTemplate   = ".claude/skills/ui-mockup/assets/mockup-template.html"
	flyoutTemplate   = ".claude/skills/ui-mockup/assets/flyout-template.html"
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
	{"a receiver row", flyoutGo, `ui.Row(c).Height(deviceRowHeight).Padding(9, 12).Gap(10)`, ".fdev{height:66px"},
	{"the receiver padding", flyoutGo, `ui.Row(c).Height(deviceRowHeight).Padding(9, 12).Gap(10)`, ".fdev{height:66px;display:flex;align-items:center;gap:10px;padding:9px 12px"},
	{"the app name size", flyoutGo, `ui.Text(c, locales.AppName()).FontSize(13)`, ".fly-app{font-size:13px"},
	{"the subtitle size", flyoutGo, `ui.Text(c, locales.AppSubtitle()).FontSize(8.5)`, ".fly-sub{font-size:8.5px"},
	{"the subtitle tracking", flyoutGo, `ui.Text(c, locales.AppSubtitle()).FontSize(8.5).FontWeight(600).LetterSpacing(1.1)`, ".fly-sub{font-size:8.5px;font-weight:600;letter-spacing:1.1px"},
	{"the section label size", uiKitGo, `ui.Text(c, label).FontSize(11.5).FontWeight(600).LetterSpacing(0.55)`, ".fly-slabel{font-size:11.5px"},
	{"the section label tracking", uiKitGo, `ui.Text(c, label).FontSize(11.5).FontWeight(600).LetterSpacing(0.55)`, "letter-spacing:.55px"},
}

// sketchToken maps a sketch template's --kebab-case property to the palette
// field it copies. The template owns the rendering; the app owns the value.
type sketchToken struct{ web, field string }

var sketchPalette = []sketchToken{
	{"bg", "bg"}, {"panel", "panel"}, {"card", "card"}, {"card-hover", "cardHover"},
	{"pill", "pill"}, {"pill-hover", "pillHover"}, {"line", "line"}, {"line2", "line2"},
	{"fg", "fg"}, {"fg2", "fg2"}, {"ctl", "ctl"}, {"muted", "muted"}, {"faint", "faint"},
	{"accent", "accent"}, {"accent-soft", "accentSoft"}, {"accent-fg", "accentFg"},
	{"sel", "sel"}, {"green", "green"}, {"green-soft", "greenSoft"},
	{"red", "red"}, {"red-soft", "redSoft"},
}

// flyoutPaletteExtra is the warning pair the flyout's credential bar uses. The
// settings sketch does not draw that surface, so it does not carry the pair.
var flyoutPaletteExtra = []sketchToken{{"warning", "warning"}, {"warning-soft", "warningSoft"}}

// sketchMetric ties one declaration in the view code to the template rule that
// copied it. anchorFile is the template that carries the rule.
type sketchMetric struct{ name, goFile, goContext, anchorFile, cssAnchor string }

var sketchMetrics = []sketchMetric{
	// The settings sketch, mockup-template.html.
	{"the sidebar width", settingsWindowGo, `ui.Column(c).Width(190).FillHeight().Background(p.bg)`, mockupTemplate, "width: 190px; flex: none; display: flex; flex-direction: column;"},
	{"the title bar height", settingsWindowGo, `ui.Row(c).Height(max(bar.Height, 32))`, mockupTemplate, "height: 32px; flex: none; display: flex; align-items: center; gap: 8px;"},
	{"the title bar mark", settingsWindowGo, `ui.Icon(c, iconMark).Size(14, 14)`, mockupTemplate, ".titlebar .mark { width: 14px; height: 14px;"},
	{"the sidebar item height", settingsWindowGo, `ui.ButtonBase(c.Key(page)).Height(32).Padding(0, 9).Gap(8).Radius(7)`, mockupTemplate, "height: 32px; padding: 0 9px; border-radius: 7px;"},
	{"the sidebar icon", settingsWindowGo, `ui.Icon(c, icon).Size(15, 15)`, mockupTemplate, ".nav-item .icon { width: 15px; height: 15px;"},
	{"the page title size", settingsWindowGo, `ui.Text(c, title).FontSize(18).FontWeight(650)`, mockupTemplate, ".page-title { font-size: 18px; font-weight: 650; }"},
	{"the card head height", uiKitGo, `ui.Row(c).Height(44).Padding(0, padX).Gap(8)`, mockupTemplate, "height: 44px; padding: 0 14px;"},
	{"the card head tile", uiKitGo, `ui.Box(c).Size(22, 22).Radius(6).Shrink(0).Center()`, mockupTemplate, "width: 22px; height: 22px; flex: none; border-radius: 6px;"},
	{"a settings row", uiKitGo, `ui.Row(c).MinHeight(56).Padding(8, 14).Gap(12)`, mockupTemplate, "min-height: 56px; padding: 8px 14px;"},
	{"the settings row title", uiKitGo, `ui.Text(c, title).FontSize(13).FontWeight(500)`, mockupTemplate, ".setting-copy .title { font-size: 13px; font-weight: 500;"},
	{"the settings row subtitle", uiKitGo, `ui.Text(c, sub).FontSize(11.5)`, mockupTemplate, ".setting-copy .sub { font-size: 11.5px;"},
	{"the settings card radius", settingsWindowGo, `ui.Column(c).Margin(12, 0, 0, 0).Radius(12).Background(p.card)`, mockupTemplate, "border-radius: 12px;"},
	{"the sidebar identity dot", connectionPageGo, `ui.Box(c).Size(6, 6).Radius(3).Background(tone)`, mockupTemplate, ".identity-dot { width: 6px; height: 6px; flex: none; border-radius: 3px;"},
	{"the sidebar identity name", connectionPageGo, `ui.Text(c, name).FontSize(11.5).SingleLine().Ellipsis("…")`, mockupTemplate, ".identity-copy .name { font-size: 11.5px;"},
	{"the sidebar identity subtitle", connectionPageGo, `ui.Text(c, sub).FontSize(10.5).TextColor(p.muted).SingleLine().Ellipsis("…")`, mockupTemplate, ".identity-copy .sub { font-size: 10.5px;"},

	// The flyout sketch, flyout-template.html.
	{"the header height", flyoutGo, `ui.Row(c).Height(46).Padding(0, 10, 0, 14).Gap(8)`, flyoutTemplate, "height: 46px; flex: none;"},
	{"the header padding", flyoutGo, `ui.Row(c).Height(46).Padding(0, 10, 0, 14).Gap(8)`, flyoutTemplate, "padding: 0 10px 0 14px;"},
	{"the app mark", flyoutGo, `ui.Icon(c, iconMark).Size(21, 21)`, flyoutTemplate, ".flyout-head .mark { width: 21px; height: 21px;"},
	{"an icon button", uiKitGo, `ui.ButtonBase(c).Size(26, 26)`, flyoutTemplate, "width: 26px; height: 26px; border-radius: 7px;"},
	{"the app name size", flyoutGo, `ui.Text(c, locales.AppName()).FontSize(13).FontWeight(600)`, flyoutTemplate, ".app-name { font-size: 13px; font-weight: 600;"},
	{"the subtitle size", flyoutGo, `ui.Text(c, locales.AppSubtitle()).FontSize(8.5).FontWeight(600).LetterSpacing(1.1)`, flyoutTemplate, ".app-sub { font-size: 8.5px; font-weight: 600; letter-spacing: 1.1px;"},
	{"the section label", uiKitGo, `ui.Text(c, label).FontSize(11.5).FontWeight(600).LetterSpacing(0.55)`, flyoutTemplate, ".section-label { font-size: 11.5px; font-weight: 600; letter-spacing: .55px;"},
	{"a receiver row", flyoutGo, `ui.Row(c).Height(deviceRowHeight).Padding(9, 12).Gap(10)`, flyoutTemplate, "height: 66px; display: flex; align-items: center; gap: 10px;"},
	{"the receiver padding", flyoutGo, `ui.Row(c).Height(deviceRowHeight).Padding(9, 12).Gap(10)`, flyoutTemplate, "padding: 9px 12px;"},
	{"the empty card height", flyoutHeightGo, `deviceEmptyHeight = 141`, flyoutTemplate, "height: 141px; display: flex; flex-direction: column;"},
	{"the empty card padding", flyoutGo, `ui.Column(c).Fill().Center().Gap(4).Padding(18)`, flyoutTemplate, "align-items: center; justify-content: center; gap: 4px; padding: 18px;"},
	{"the media card row", flyoutGo, `ui.Row(c).Height(62).Gap(10).AlignItems(ui.Center).Padding(6, 0, 4, 0)`, flyoutTemplate, "height: 62px; display: flex; align-items: center; gap: 10px; padding: 6px 0 4px 0;"},
	{"the media title size", flyoutGo, `ui.Text(c, title).FontSize(15).FontWeight(500)`, flyoutTemplate, ".media-title { font-size: 15px; font-weight: 500;"},
	{"the footer height", flyoutGo, `ui.Row(c).Height(34).Padding(4, 12, 6, 14).Gap(6)`, flyoutTemplate, "height: 34px; flex: none; display: flex; align-items: center; gap: 6px;"},
	{"the footer padding", flyoutGo, `ui.Row(c).Height(34).Padding(4, 12, 6, 14).Gap(6)`, flyoutTemplate, "padding: 4px 12px 6px 14px;"},
}

// heightConstant pins one number in the flyout sketch's height rule to the
// constant that derives it.
type heightConstant struct{ goName, goFile, templateName string }

var heightConstants = []heightConstant{
	{"flyoutHeight", flyoutHeightGo, "FLYOUT_BASE"},
	{"deviceRows", flyoutHeightGo, "DEVICE_ROWS"},
	{"deviceRowHeight", flyoutHeightGo, "DEVICE_ROW_H"},
	{"deviceEmptyHeight", flyoutHeightGo, "DEVICE_EMPTY"},
	{"nowPlayingRowHeight", flyoutHeightGo, "NOW_ROW_H"},
	{"nowPlayingCardHeight", flyoutHeightGo, "MEDIA_H"},
	{"latencySwitchHeight", flyoutHeightGo, "LATENCY_H"},
	{"errorBarHeight", flyoutHeightGo, "ERROR_H"},
	{"lampSectionHeight", lampGo, "LAMP_H"},
}

var (
	goPair      = regexp.MustCompile(`([A-Za-z0-9]+)\s*:\s*ui\.Hex\("#([0-9a-fA-F]{6})"\)`)
	cssToken    = regexp.MustCompile(`--fl-([A-Za-z0-9]+):\s*(#[0-9a-fA-F]{3,8})`)
	templateVar = regexp.MustCompile(`--([A-Za-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{3,8})`)
	bodyRule    = regexp.MustCompile(`(?m)^\s*body\s*\{([^}]*)\}`)
	bodyDark    = regexp.MustCompile(`(?m)^\s*body\.dark\s*\{([^}]*)\}`)
)

// runTokens reads the sources and reports every way the copies have parted
// from the app's own values.
func runTokens(root string) error {
	read := func(rel string) (string, error) {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	needed := []string{siteHTML, themeGo, flyoutGo, flyoutHeightGo, uiKitGo, lampGo, settingsWindowGo, connectionPageGo, settingsCreateGo, mockupTemplate, flyoutTemplate}
	sources := map[string]string{}
	for _, rel := range needed {
		src, err := read(rel)
		if err != nil {
			return err
		}
		sources[rel] = src
	}
	// The flyout's geometry constants live in flyout_height.go; the metrics
	// that pin them read one combined source so a move across files is not a
	// drift.
	sources[flyoutGo] = sources[flyoutGo] + "\n" + sources[flyoutHeightGo]
	sources[flyoutHeightGo] = sources[flyoutGo]

	problems, width := checkTokens(sources[siteHTML], sources[themeGo], sources[flyoutGo], sources[uiKitGo])
	sketchProblems, sketchChecked := checkSketches(sources[themeGo], sources[mockupTemplate], sources[flyoutTemplate], sources)
	problems = append(problems, sketchProblems...)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Println("check-ui tokens:", p)
		}
		return fmt.Errorf("tokens: %d drift(s)", len(problems))
	}
	fmt.Printf("check-ui: site %d tokens in two appearances, %d metrics, the panel at %s px\n",
		len(tokens), len(metrics), width)
	fmt.Printf("check-ui: sketches %d tokens in two appearances, %d metric values and %d height-rule values across 2 templates\n",
		sketchChecked.tokens, sketchChecked.metrics, sketchChecked.heights)
	fmt.Println("check-ui: the replica and both sketch templates still follow the app")
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

// sketchChecks is what the pass line reports.
type sketchChecks struct{ tokens, metrics, heights int }

// checkSketches compares both templates' palette blocks, their checked
// metrics, the flyout height rule, and the drawn window sizes with the app.
func checkSketches(theme, mockup, sketchFlyout string, sources map[string]string) (problems []string, checked sketchChecks) {
	appDark, appLight := palettes(theme)
	common := sketchPalette
	flyoutAll := append(append([]sketchToken{}, common...), flyoutPaletteExtra...)

	for _, t := range []struct {
		file     string
		label    string
		src      string
		required []sketchToken
	}{
		{mockupTemplate, "mockup-template.html", mockup, common},
		{flyoutTemplate, "flyout-template.html", sketchFlyout, flyoutAll},
	} {
		light, dark, ok := templatePalettes(t.src)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s no longer carries a light body rule and a body.dark rule; the palette block moved", t.label))
			continue
		}
		for _, tok := range t.required {
			for _, appearance := range []struct {
				label    string
				app, web map[string]string
			}{
				{"dark", appDark, dark},
				{"light", appLight, light},
			} {
				a, ok := appearance.app[tok.field]
				if !ok {
					problems = append(problems, fmt.Sprintf(
						"%s: internal/app/theme.go no longer declares %q, so --%s answers to nothing",
						appearance.label, tok.field, tok.web))
					continue
				}
				w, ok := appearance.web[tok.web]
				if !ok {
					problems = append(problems, fmt.Sprintf(
						"%s: %s declares no --%s for the app's %s", appearance.label, t.label, tok.web, a))
					continue
				}
				if !strings.EqualFold(a, w) {
					problems = append(problems, fmt.Sprintf(
						"%s: %s sets --%s to %s and theme.go sets %s to %s",
						appearance.label, t.label, tok.web, w, tok.field, a))
				}
			}
		}
		checked.tokens += len(t.required)
	}

	anchors := map[string]string{mockupTemplate: mockup, flyoutTemplate: sketchFlyout}
	for _, m := range sketchMetrics {
		checked.metrics++
		if !strings.Contains(sources[m.goFile], m.goContext) {
			problems = append(problems, fmt.Sprintf(
				"%s no longer declares %s where the sketch copies it; update the template and this check", m.goFile, m.name))
		}
		if !strings.Contains(anchors[m.anchorFile], m.cssAnchor) {
			problems = append(problems, fmt.Sprintf(
				"%s no longer carries %q; the view still declares %s", m.anchorFile, m.cssAnchor, m.name))
		}
	}

	heightProblems, heightCount := checkHeightRule(sources[flyoutGo], sources[lampGo], sketchFlyout)
	problems = append(problems, heightProblems...)
	checked.heights += heightCount

	sizeProblems, sizeCount := checkSketchSizes(sources[flyoutGo], sources[settingsCreateGo], sketchFlyout, mockup)
	problems = append(problems, sizeProblems...)
	checked.heights += sizeCount

	return problems, checked
}

// templatePalettes reads the light body rule and the dark override. The
// templates keep the whole design palette in those two rules.
func templatePalettes(src string) (light, dark map[string]string, ok bool) {
	l := bodyRule.FindStringSubmatch(src)
	d := bodyDark.FindStringSubmatch(src)
	if l == nil || d == nil {
		return nil, nil, false
	}
	return templatePairs(l[1]), templatePairs(d[1]), true
}

func templatePairs(block string) map[string]string {
	out := map[string]string{}
	for _, m := range templateVar.FindAllStringSubmatch(block, -1) {
		out[m[1]] = m[2]
	}
	return out
}

// checkHeightRule pins the sketch's flyoutHeight function to the constants
// that derive it on the app side.
func checkHeightRule(flyout, lamp, template string) (problems []string, checked int) {
	byFile := map[string]string{flyoutGo: flyout, flyoutHeightGo: flyout, lampGo: lamp}
	for _, c := range heightConstants {
		checked++
		goVal, ok := digitsIn(byFile[c.goFile], `\b`+c.goName+`\s*=\s*(\d+)`)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s no longer declares %s; update the sketch's height rule and this check", c.goFile, c.goName))
			continue
		}
		webVal, ok := digitsIn(template, `\b`+c.templateName+`\s*=\s*(\d+)`)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s no longer declares %s; the app declares %s %s", flyoutTemplate, c.templateName, c.goName, goVal))
			continue
		}
		if goVal != webVal {
			problems = append(problems, fmt.Sprintf(
				"%s: %s is %s and %s is %s", flyoutTemplate, c.templateName, webVal, c.goName, goVal))
		}
	}

	checks := []struct {
		name     string
		goSrc    string
		goExpr   string
		template string
		webExpr  string
	}{
		{"the device card border", flyout, `deviceRowHeight \+ (\d+)`, template, `DEVICE_ROW_H \+ (\d+)`},
		{"the lamp grid columns", lamp, `lampColumns\s*=\s*(\d+)`, template, `Math\.ceil\(count / (\d+)\)`},
		{"the visible lamp rows", lamp, `lampRowsVisible\s*=\s*(\d+)`, template, `Math\.max\(Math\.ceil\(count / \d+\), 1\), (\d+)\)`},
		{"the lamp swatch size", lamp, `lampSwatchSize\s*=\s*(\d+)`, template, `rows \* (\d+) \+ \(rows - 1\)`},
		{"the lamp row gap", lamp, `lampSwatchGapY\s*=\s*(\d+)`, template, `\(rows - 1\) \* (\d+) \+`},
		{"the lamp grid padding", lamp, `lampGridPadding\s*=\s*(\d+)`, template, `\+ (\d+) \+ 1`},
	}
	for _, c := range checks {
		checked++
		goVal, ok := digitsIn(c.goSrc, c.goExpr)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"internal/app no longer declares %s; update the sketch's height rule and this check", c.name))
			continue
		}
		webVal, ok := digitsIn(c.template, c.webExpr)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s no longer computes %s; the app declares %s", flyoutTemplate, c.name, goVal))
			continue
		}
		if goVal != webVal {
			problems = append(problems, fmt.Sprintf(
				"%s: %s is %s and the app declares %s", flyoutTemplate, c.name, webVal, goVal))
		}
	}
	return problems, checked
}

// checkSketchSizes pins the windows the templates draw to the sizes the app
// opens them at.
func checkSketchSizes(flyout, settings, templateFlyout, mockup string) (problems []string, checked int) {
	pairs := []struct {
		name, goSrc, goExpr, label, src, webExpr string
	}{
		{"the flyout width", flyout, `flyoutWidth\s*=\s*(\d+)`, "flyout-template.html", templateFlyout, `(?s)\.flyout\s*\{[^}]*?\bwidth:\s*(\d+)px`},
		{"the settings width", settings, `settingsContentWidth\s*=\s*(\d+)`, "mockup-template.html", mockup, `(?s)\.window\s*\{[^}]*?\bwidth:\s*(\d+)px`},
		{"the settings height", settings, `settingsContentHeight\s*=\s*(\d+)`, "mockup-template.html", mockup, `(?s)\.window\s*\{[^}]*?\bheight:\s*(\d+)px`},
	}
	for _, p := range pairs {
		checked++
		goVal, ok := digitsIn(p.goSrc, p.goExpr)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"internal/app no longer declares %s; update the template and this check", p.name))
			continue
		}
		webVal, ok := digitsIn(p.src, p.webExpr)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s no longer draws %s; the app uses %s", p.label, p.name, goVal))
			continue
		}
		if goVal != webVal {
			problems = append(problems, fmt.Sprintf(
				"%s draws %s at %s and the app uses %s", p.label, p.name, webVal, goVal))
		}
	}
	return problems, checked
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
