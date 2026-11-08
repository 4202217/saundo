package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/egoist/mygo/i18n"

	"saundo/cmd/saundo/locales"
)

// The repository the updater is built with (updates.github in mygo.json).
// The notes dialog reads the localized changelog for the versions between
// the running build and the offered one; the signed manifest stays the
// source for the newest notes.
const (
	updateRepo        = "4202217/saundo"
	updateChangelog   = "https://raw.githubusercontent.com/" + updateRepo + "/main/CHANGELOG.md"
	updateChangelogZH = "https://raw.githubusercontent.com/" + updateRepo + "/main/CHANGELOG.zh-CN.md"
)

func changelogURL(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "zh") {
		return updateChangelogZH
	}
	return updateChangelog
}

// releaseNote is one version's release notes.
type releaseNote struct {
	Version string
	Date    time.Time
	Notes   string
}

type updateHistoryState uint8

const (
	historyIdle updateHistoryState = iota
	historyLoading
	historyReady
	historyFailed
)

// loadReleaseHistory asks for the versions between the running build and
// the offered one, once per offered version and app language. A failure
// keeps the dialog on the manifest's notes and offers the GitHub page.
func (a *App) loadReleaseHistory() {
	if a.upd.version == "" || a.upd.up == nil {
		return
	}
	locale := i18n.Locale()
	if a.historyKey == a.upd.version && a.historyLocale == locale &&
		(a.historyState == historyLoading || a.historyState == historyReady) {
		return
	}
	latest := a.upd.version
	current := a.currentVersion()
	a.historyKey, a.historyLocale = latest, locale
	a.historyState = historyLoading
	a.history = nil
	a.runAsync(func() {
		notes, err := fetchReleaseNotes(locale, current, latest)
		a.updateUI(func() {
			if a.historyKey != latest || a.historyLocale != locale {
				return
			}
			if err != nil {
				a.historyState = historyFailed
				a.log.Printf("update: release notes: %v", err)
				return
			}
			a.history, a.historyState = notes, historyReady
		})
	})
}

func fetchReleaseNotes(locale, current, latest string) ([]releaseNote, error) {
	url := changelogURL(locale)
	text, err := fetchText(url)
	if err != nil {
		text, err = fetchText(strings.Replace(url, "/main/", "/master/", 1))
		if err != nil {
			return nil, err
		}
	}
	return filterNotes(current, latest, parseChangelog(text)), nil
}

func fetchText(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "saundo")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("changelog: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseChangelog splits the changelog at its "## <version> - <date>"
// headings.
func parseChangelog(text string) []releaseNote {
	var out []releaseNote
	var cur *releaseNote
	var body strings.Builder
	flush := func() {
		if cur == nil {
			return
		}
		cur.Notes = strings.TrimSpace(body.String())
		out = append(out, *cur)
		body.Reset()
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			version, date := title, time.Time{}
			if i := strings.Index(title, " - "); i > 0 {
				version = strings.TrimSpace(title[:i])
				if t, err := time.Parse("2006-01-02", strings.TrimSpace(title[i+3:])); err == nil {
					date = t
				}
			}
			cur = &releaseNote{Version: strings.TrimPrefix(version, "v"), Date: date}
			continue
		}
		if cur != nil {
			body.WriteString(line)
			body.WriteString("\n")
		}
	}
	flush()
	return out
}

// filterNotes keeps the versions newer than current and no newer than
// latest, newest first.
func filterNotes(current, latest string, notes []releaseNote) []releaseNote {
	latestV := normalizeVersion(latest)
	curV := normalizeVersion(current)
	out := make([]releaseNote, 0, len(notes))
	for _, n := range notes {
		v := normalizeVersion(n.Version)
		if !semver.IsValid(v) {
			continue
		}
		if semver.IsValid(latestV) && semver.Compare(v, latestV) > 0 {
			continue
		}
		if semver.IsValid(curV) && semver.Compare(v, curV) <= 0 {
			continue
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		return semver.Compare(normalizeVersion(out[i].Version), normalizeVersion(out[j].Version)) > 0
	})
	return out
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if v == "" {
		return ""
	}
	return "v" + v
}

// summarizeNotes names the section with the most entries, for the folded
// rows: fixed, changed, or added.
func summarizeNotes(notes string) (string, int) {
	counts := map[string]int{}
	section := ""
	total := 0
	for _, line := range strings.Split(notes, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			h := strings.ToLower(strings.TrimSpace(strings.TrimLeft(t, "#")))
			switch {
			case strings.Contains(h, "added") || strings.Contains(h, "新增") || strings.Contains(h, "新功能"):
				section = "added"
			case strings.Contains(h, "changed") || strings.Contains(h, "变更") || strings.Contains(h, "改进"):
				section = "changed"
			case strings.Contains(h, "fixed") || strings.Contains(h, "修复"):
				section = "fixed"
			default:
				section = ""
			}
			continue
		}
		if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
			total++
			if section != "" {
				counts[section]++
			}
		}
	}
	best, bestN := "", 0
	for _, kind := range []string{"fixed", "changed", "added"} {
		if counts[kind] > bestN {
			best, bestN = kind, counts[kind]
		}
	}
	if best == "" {
		return "updated", total
	}
	return best, bestN
}

func noteCountLabel(kind string, n int) string {
	switch kind {
	case "added":
		return locales.UpdateCountAdded(n)
	case "changed":
		return locales.UpdateCountChanged(n)
	case "fixed":
		return locales.UpdateCountFixed(n)
	}
	return locales.UpdateCountUpdated(n)
}

// notesEntries is the dialog's list: the offered version, then the versions
// fetched from the release feed. The manifest carries the English notes; a
// fetched entry for the offered version is in the app language and replaces
// them.
func (a *App) notesEntries() []releaseNote {
	offered := normalizeVersion(a.upd.version)
	entries := []releaseNote{{Version: a.upd.version, Date: a.upd.date, Notes: a.upd.notes}}
	seen := map[string]bool{offered: true}
	for _, h := range a.history {
		key := normalizeVersion(h.Version)
		if seen[key] {
			if key == offered && strings.TrimSpace(h.Notes) != "" {
				entries[0].Notes = h.Notes
			}
			continue
		}
		seen[key] = true
		entries = append(entries, h)
	}
	if !a.notesRange {
		return entries[:1]
	}
	return entries
}

// notesRangeMeta is the dialog's subtitle: the version span, or the date of
// a single version.
func (a *App) notesRangeMeta() string {
	entries := a.notesEntries()
	if a.notesRange && len(entries) > 1 {
		return locales.UpdateNotesRangeMeta(
			entries[len(entries)-1].Version,
			entries[0].Version,
			len(entries),
			a.currentVersionLabel(),
		)
	}
	return locales.UpdateNotesMeta(updateDate(a.upd.date), a.currentVersionLabel())
}
