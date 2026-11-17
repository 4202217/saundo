//go:build preview

package app

// The landing page's flyout replica, checked against the words the real flyout
// renders. Ported from the render-demo research branch at f3f6bf8.

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// replicaTextGaps are messages the flyout shows that the landing page does not
// carry yet. The list only ever shrinks: when the page gains a message, this
// test fails until the entry is removed, so a gap cannot be closed and left
// declared.
var replicaTextGaps = []string{
	"媒体",                   // nor the lamp's media option
	"颜色来源", "Color source", // nor a label on the lamp row's segments
	"Show colors",   // nor a name for the lamp's disclosure button, in English
	"Receiver list", // the receiver list is unlabelled for a screen reader, in English
}

// TestReplicaSharesTheFlyoutsWording keeps the landing page's hand-built
// replica honest about words. Every message the catalogs own that the real
// flyout actually renders has to appear on the page too. The demand comes
// from the rendered view rather than from a table someone maintains, so a
// label joins this check the day it ships.
//
// The page is allowed its own story — receivers, rooms, track, volume — which
// is why the check runs this way round: what the flyout shows is the demand,
// the page is the supply.
func TestReplicaSharesTheFlyoutsWording(t *testing.T) {
	old := testLocale
	defer func() { testLocale = old }()

	page := pageText(t, "../../site/index.html")

	declared := map[string]bool{}
	for _, gap := range replicaTextGaps {
		declared[gap] = true
	}

	var missing, fixed []string
	demanded := map[string]bool{}
	shown := 0
	for _, locale := range []string{"zh-CN", "en"} {
		testLocale = locale
		a := replicaStreamingApp(t, locale)
		tt := ui.NewTester(a.view, flyoutWidth, visibleFlyoutHeight(a))
		tt.SetDark(true)
		tt.Frame()
		rendered := strings.Join(tt.Texts(), "\n")
		catalog := catalogStrings(t, "../../cmd/saundo/locales/"+locale+".json")
		data := fixtureText(a)

		for _, msg := range catalog {
			if !shows(rendered, msg) || shows(data, msg) {
				// The flyout does not use this message, or it happens to render
				// a fixture value that reads like one.
				continue
			}
			shown++
			if declared[msg] {
				demanded[msg] = true
			}
			switch {
			case pageShows(page, msg) && declared[msg]:
				fixed = append(fixed, msg)
			case !pageShows(page, msg) && !declared[msg]:
				missing = append(missing, msg)
			}
		}
	}
	if shown == 0 {
		t.Fatal("the flyout rendered none of the catalog's messages; the fixture or the catalog moved")
	}
	if len(fixed) > 0 {
		t.Fatalf("the page now carries %s; drop those entries from replicaTextGaps",
			strings.Join(fixed, ", "))
	}
	if len(missing) > 0 {
		t.Fatalf("the page is missing %d of the %d messages the flyout shows:\n  %s",
			len(missing), shown, strings.Join(missing, "\n  "))
	}
	var stale []string
	for _, gap := range replicaTextGaps {
		if !demanded[gap] {
			stale = append(stale, gap)
		}
	}
	if len(stale) > 0 {
		t.Fatalf("replicaTextGaps is stale: the rendered flyout no longer shows %s",
			strings.Join(stale, ", "))
	}
	t.Logf("%d messages the flyout shows: %d on the page, %d declared as gaps",
		shown, shown-len(replicaTextGaps), len(replicaTextGaps))
}

// pageShows reports whether the page supplies msg. Most messages are matched
// as substrings, as the rendered view and the markup split words differently.
// The one-word lamp option 媒体 needs a boundary: 切换媒体 now names the
// session-switch control, and its label is not the lamp's media option.
func pageShows(text, msg string) bool {
	if msg == "媒体" {
		return regexp.MustCompile(`(?:^|[^\p{Han}])` + regexp.QuoteMeta(msg) + `(?:$|[^\p{Han}])`).MatchString(text)
	}
	return shows(text, msg)
}

// shows reports whether text carries msg. Placeholders stand for values, so a
// message such as "正在通过 {name} 播放" becomes "正在通过 .* 播放"; whitespace is
// ignored on both sides, because markup and templates split words differently
// from the way the view paints them.
func shows(text, msg string) bool {
	var b strings.Builder
	parts := placeholder.Split(msg, -1)
	literal := false
	for i, part := range parts {
		if i > 0 {
			b.WriteString(".*")
		}
		part = squeeze(part)
		if part != "" {
			literal = true
		}
		b.WriteString(regexp.QuoteMeta(part))
	}
	if !literal {
		return false
	}
	return regexp.MustCompile(b.String()).MatchString(squeeze(text))
}

var (
	placeholder = regexp.MustCompile(`\{[^}]*\}`)
	scriptBody  = regexp.MustCompile(`(?is)<script[^>]*>(.*?)</script>`)
	attrValue   = regexp.MustCompile(`(?is)\b(?:aria-label|alt|title)\s*=\s*"([^"]*)"`)
	modelLine   = regexp.MustCompile(`\.model\s*\+\s*" · "\s*\+\s*[^;]{0,120}?t\("([^"]+)"\)`)
)

// squeeze drops every space so that a message compares the same whether it was
// split across elements, written in a template, or painted by the view.
func squeeze(s string) string {
	return strings.Join(strings.Fields(s), "")
}

// catalogStrings is every message in the catalog, whatever its key.
func catalogStrings(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Keys that start with @ describe a message's placeholders instead of
	// carrying one, so they hold objects rather than strings.
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	out := make([]string, 0, len(doc))
	for key, raw := range doc {
		if strings.HasPrefix(key, "@") {
			continue
		}
		var v string
		if json.Unmarshal(raw, &v) != nil || v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

// fixtureText is the demo's own data — device names, models, the track, the
// player. A message that also reads like one of these values says nothing
// about the page.
func fixtureText(a *App) string {
	var b strings.Builder
	b.WriteString(a.snap.Name + "\n")
	for _, d := range a.snap.Devices {
		b.WriteString(d.Name + "\n" + d.Model + "\n")
	}
	for _, s := range a.nowSnap.Sessions {
		b.WriteString(s.AppName + "\n" + s.Title + "\n" + s.Artist + "\n" + s.Album + "\n")
	}
	b.WriteString(a.nowTrack.Title + "\n" + a.nowTrack.Artist + "\n" + a.nowTrack.Album + "\n")
	return b.String()
}

// pageText is the text the page carries: its markup and its scripts, with the
// tags, styles and drawings taken out. Attribute-only copy (aria-label, alt,
// title) is collected first, because a wordless control still names itself to
// a screen reader. The demo builds some strings in the script, so stripping
// scripts would report the page missing words it shows.
func pageText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	var attrs []string
	for _, m := range attrValue.FindAllStringSubmatch(s, -1) {
		attrs = append(attrs, m[1])
	}
	// Scripts are lifted out before the tags are stripped: a comparison like
	// `i < n` in JavaScript would otherwise look like the start of a tag and
	// swallow the strings that follow it.
	var scripts []string
	for _, m := range scriptBody.FindAllStringSubmatch(s, -1) {
		scripts = append(scripts, m[1])
	}
	s = scriptBody.ReplaceAllString(s, " ")
	for _, tag := range []string{"style", "svg"} {
		s = regexp.MustCompile(`(?is)<`+tag+`[^>]*>.*?</`+tag+`>`).ReplaceAllString(s, " ")
	}
	s = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(s, " ")
	s += "\n" + strings.Join(attrs, "\n") + "\n" + strings.Join(scripts, "\n")
	s += "\n" + strings.Join(composedPageLines(scripts), "\n")
	for _, pair := range [][2]string{
		{"&nbsp;", " "}, {"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"}, {"&quot;", `"`}, {"&#39;", "'"},
	} {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	return s
}

// composedPageLines supplies the receiver subtitles the page builds from a
// shape and a label: `d.model + " · " + t("PIN 配对")` paints the same message
// the flyout does, but the text holds the pieces. Each composition is added in
// the language whose label the page owns, so both catalogs see what it paints.
func composedPageLines(scripts []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(line string) {
		if line != "" && !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	for _, script := range scripts {
		for _, m := range modelLine.FindAllStringSubmatch(script, -1) {
			label := m[1]
			add("{model} · " + label)
			if en := pageTranslation(script, label); en != "" {
				add("{model} · " + en)
			}
		}
	}
	return out
}

// pageTranslation reads the English value the page's own dictionary gives a
// Chinese label, so a composed line follows the copy the page actually paints.
func pageTranslation(script, key string) string {
	m := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*"([^"]*)"`).FindStringSubmatch(script)
	if m == nil {
		return ""
	}
	return m[1]
}
