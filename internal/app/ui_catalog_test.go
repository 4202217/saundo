//go:build preview

package app

import (
	"sort"
	"strings"
	"testing"
)

// TestCatalog proves the table's shape: unique IDs and stems, known surfaces,
// one view per scenario, non-nil Apply and Size, a positive size after Apply
// runs on a fresh fixture, and a site projection that names real catalog rows.
func TestCatalog(t *testing.T) {
	if len(Catalog()) == 0 {
		t.Fatal("catalog is empty")
	}
	ids := map[string]bool{}
	stems := map[string]bool{}
	siteDemoIDs := map[string]bool{}
	siteDemo := 0
	for _, sc := range Catalog() {
		if sc.ID == "" {
			t.Fatalf("%s: empty ID", sc.Name)
		}
		if ids[sc.ID] {
			t.Fatalf("duplicate ID %q", sc.ID)
		}
		ids[sc.ID] = true
		if sc.Name == "" {
			t.Fatalf("%s: empty Name", sc.ID)
		}
		if stems[sc.Name] {
			t.Fatalf("%s: duplicate stem %q", sc.ID, sc.Name)
		}
		stems[sc.Name] = true
		if sc.Surface > SurfaceTrayMenu {
			t.Fatalf("%s: unknown surface %d", sc.ID, sc.Surface)
		}
		if sc.Surface == SurfaceTrayMenu {
			t.Fatalf("%s: the tray menu has no CPU view yet", sc.ID)
		}
		if sc.SiteDemo {
			siteDemo++
			siteDemoIDs[sc.ID] = true
			if sc.Surface != SurfaceFlyout {
				t.Fatalf("%s: SiteDemo is a flyout projection, not surface %d", sc.ID, sc.Surface)
			}
		}
		if sc.Surface == SurfaceDialog {
			if sc.Host != SurfaceFlyout && sc.Host != SurfaceSettings {
				t.Fatalf("%s: dialog host %d is not a view", sc.ID, sc.Host)
			}
		} else if sc.Host != SurfaceFlyout {
			t.Fatalf("%s: Host is only for dialogs", sc.ID)
		}
		if sc.Apply == nil {
			t.Fatalf("%s: nil Apply", sc.ID)
		}
		if sc.Size == nil {
			t.Fatalf("%s: nil Size", sc.ID)
		}
		switch sc.Theme {
		case ThemeBoth:
			if sc.Dark == "" {
				t.Fatalf("%s: ThemeBoth needs a dark stem", sc.ID)
			}
			if stems[sc.Dark] {
				t.Fatalf("%s: duplicate stem %q", sc.ID, sc.Dark)
			}
			stems[sc.Dark] = true
		case ThemeLight, ThemeDark:
			if sc.Dark != "" {
				t.Fatalf("%s: Dark is only for ThemeBoth", sc.ID)
			}
		default:
			t.Fatalf("%s: unknown theme %d", sc.ID, sc.Theme)
		}
		if sc.Frames != 0 && sc.Frames != 3 {
			t.Fatalf("%s: Frames %d is not 0 or 3", sc.ID, sc.Frames)
		}
		if sc.Frames > 0 && (sc.Scale != 0 || sc.Scroll != 0 || sc.Theme != ThemeLight) {
			t.Fatalf("%s: frames take no scale, scroll, or dark theme", sc.ID)
		}
		if sc.Scale < 0 || sc.Scroll < 0 {
			t.Fatalf("%s: negative scale or scroll", sc.ID)
		}
		if sc.usesSettingsPage() && sc.Page > pageAbout {
			t.Fatalf("%s: unknown settings page %d", sc.ID, sc.Page)
		}
		a := Fixture("en")
		if sc.usesSettingsPage() {
			a.page = sc.Page
		}
		sc.Apply(a)
		if sc.view(a) == nil {
			t.Fatalf("%s: surface %d has no view", sc.ID, sc.Surface)
		}
		w, h := sc.Size(a)
		if w <= 0 || h <= 0 {
			t.Fatalf("%s: size %dx%d", sc.ID, w, h)
		}
	}
	if siteDemo == 0 {
		t.Fatal("no SiteDemo scenario: the landing page has no catalog projection")
	}
	// The site dataset is the supply the page actually loads. Comparing the
	// catalog's SiteDemo rows with the file catches a stale dataset and a
	// scenario whose row was marked or unmarked without regenerating it.
	if problems := siteDemoProblems(ids, siteDemoIDs, replicaDataFromFile(t).Scenarios); len(problems) > 0 {
		t.Fatalf("the site dataset and the catalog disagree:\n  %s", strings.Join(problems, "\n  "))
	}
}

// siteDemoProblems compares the catalog's SiteDemo rows with the dataset's
// scenario list. It returns one line per disagreement, sorted, so the gate
// names the row instead of only failing.
func siteDemoProblems(ids, siteDemo map[string]bool, scenarios []string) []string {
	var problems []string
	seen := map[string]bool{}
	for _, id := range scenarios {
		if seen[id] {
			problems = append(problems, id+" is listed more than once in the site dataset")
		}
		seen[id] = true
		if !ids[id] {
			problems = append(problems, id+" is not a catalog row")
		} else if !siteDemo[id] {
			problems = append(problems, id+" is in the site dataset but is not a SiteDemo row")
		}
	}
	for id := range siteDemo {
		if !seen[id] {
			problems = append(problems, id+" is a SiteDemo row but is missing from the site dataset")
		}
	}
	sort.Strings(problems)
	return problems
}

// TestSiteDemoProblemsCatchesDrift proves the comparison can fail: a scenario
// that is missing from the file and one the catalog does not have both report.
func TestSiteDemoProblemsCatchesDrift(t *testing.T) {
	ids := map[string]bool{"flyout.stream": true, "flyout.idle": true}
	siteDemo := map[string]bool{"flyout.stream": true, "flyout.idle": true}
	if got := siteDemoProblems(ids, siteDemo, []string{"flyout.stream", "flyout.idle"}); len(got) > 0 {
		t.Fatalf("the matching pair reported %v", got)
	}
	if got := siteDemoProblems(ids, siteDemo, []string{"flyout.stream"}); len(got) == 0 {
		t.Fatal("a SiteDemo row missing from the dataset passed")
	}
	if got := siteDemoProblems(ids, siteDemo, []string{"flyout.stream", "flyout.idle", "flyout.extra"}); len(got) == 0 {
		t.Fatal("a dataset scenario with no catalog row passed")
	}
	if got := siteDemoProblems(ids, map[string]bool{"flyout.stream": true}, []string{"flyout.stream", "flyout.idle"}); len(got) == 0 {
		t.Fatal("a dataset scenario that is not SiteDemo passed")
	}
}
