package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readStatesSources(t *testing.T) (catalog, template string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", uiCatalogFlyoutGo))
	if err != nil {
		t.Fatal(err)
	}
	catalog = string(b)
	b, err = os.ReadFile(filepath.Join("..", "..", flyoutTemplate))
	if err != nil {
		t.Fatal(err)
	}
	return catalog, string(b)
}

// TestFlyoutSketchCoversCatalog is the gate itself, run as a test so a
// parser change fails beside the parser.
func TestFlyoutSketchCoversCatalog(t *testing.T) {
	catalog, template := readStatesSources(t)
	if problems := checkStates(catalog, template); len(problems) > 0 {
		t.Fatalf("the flyout sketch has drifted from the catalog:\n%s", strings.Join(problems, "\n"))
	}
}

// TestSeededCatalogScenarioFails proves the parser reads the catalog: a
// scenario the tables never named must fail.
func TestSeededCatalogScenarioFails(t *testing.T) {
	catalog, template := readStatesSources(t)
	seeded := strings.Replace(catalog, `{ID: "flyout.idle"`, `{ID: "flyout.brandNew"`, 1)
	if seeded == catalog {
		t.Fatal("the scenario seed did not apply; the catalog literal moved")
	}
	problems := checkStates(seeded, template)
	if !hasProblem(problems, "flyout.brandNew") {
		t.Fatalf("an unmapped scenario was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

// TestSeededStateDriftFails proves the parser reads the template's STATES
// block: a reach row whose state the template dropped must fail.
func TestSeededStateDriftFails(t *testing.T) {
	catalog, template := readStatesSources(t)
	seeded := strings.Replace(template, "{ id: 'connecting'", "{ id: 'linking'", 1)
	if seeded == template {
		t.Fatal("the state seed did not apply; the STATES block moved")
	}
	problems := checkStates(catalog, seeded)
	if !hasProblem(problems, `state "connecting"`) {
		t.Fatalf("a dropped state was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

// TestSeededURLDocDriftFails proves the parser reads the template's URL doc:
// a reach value the doc no longer declares must fail.
func TestSeededURLDocDriftFails(t *testing.T) {
	catalog, template := readStatesSources(t)
	seeded := strings.Replace(template, "lamp=cover|nomedia|off|template|expanded", "lamp=cover|dimmed|off|template|expanded", 1)
	if seeded == template {
		t.Fatal("the URL doc seed did not apply; the header comment moved")
	}
	problems := checkStates(catalog, seeded)
	if !hasProblem(problems, "lamp=nomedia") {
		t.Fatalf("a dropped URL value was not reported:\n%s", strings.Join(problems, "\n"))
	}
}

// TestSeededStaleRowFails proves the tables stay truthful in the other
// direction: a row the catalog no longer declares must fail.
func TestSeededStaleRowFails(t *testing.T) {
	catalog, template := readStatesSources(t)
	seeded := strings.Replace(catalog, `{ID: "flyout.lamp.off"`, `{ID: "flyout.lamp.dark"`, 1)
	if seeded == catalog {
		t.Fatal("the stale-row seed did not apply; the catalog literal moved")
	}
	problems := checkStates(seeded, template)
	if !hasProblem(problems, "flyout.lamp.off") {
		t.Fatalf("a stale reach row was not reported:\n%s", strings.Join(problems, "\n"))
	}
}
