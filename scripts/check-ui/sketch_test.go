package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sketchFixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{mockupTemplate, flyoutTemplate} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("<html>"+rel+" v1</html>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSketchWritesBaselineWorkingCopyAndOverlay(t *testing.T) {
	root := sketchFixtureRoot(t)
	if err := runSketch(root, []string{"-topic", "demo", "-surface", "settings"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, sketchRoot, "demo")
	template := readFile(t, filepath.Join(root, filepath.FromSlash(mockupTemplate)))
	if got := readFile(t, filepath.Join(dir, "baseline-settings.html")); got != template {
		t.Fatalf("baseline = %q, want the template", got)
	}
	if got := readFile(t, filepath.Join(dir, "mockup.html")); got != template {
		t.Fatalf("working copy = %q, want the template", got)
	}
	overlay := readFile(t, filepath.Join(dir, "overlay-settings.md"))
	for _, want := range []string{"# demo design overlay", "Scenario IDs", "target:", "mockup.html"} {
		if !strings.Contains(overlay, want) {
			t.Fatalf("overlay is missing %q:\n%s", want, overlay)
		}
	}
}

func TestSketchFlyoutUsesTheFlyoutTemplate(t *testing.T) {
	root := sketchFixtureRoot(t)
	if err := runSketch(root, []string{"-topic", "demo", "-surface", "flyout"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, sketchRoot, "demo")
	if _, err := os.Stat(filepath.Join(dir, "flyout.html")); err != nil {
		t.Fatalf("the flyout working copy is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mockup.html")); !os.IsNotExist(err) {
		t.Fatalf("the settings working copy was written for a flyout sketch")
	}
	if got := readFile(t, filepath.Join(dir, "baseline-flyout.html")); !strings.Contains(got, flyoutTemplate) {
		t.Fatalf("baseline = %q, want the flyout template", got)
	}
}

// TestSketchBothSurfacesShareOneTopic is the release's shape: the settings
// and flyout baselines have their own names, so one evidence folder holds
// both.
func TestSketchBothSurfacesShareOneTopic(t *testing.T) {
	root := sketchFixtureRoot(t)
	if err := runSketch(root, []string{"-topic", "release", "-surface", "settings"}); err != nil {
		t.Fatal(err)
	}
	if err := runSketch(root, []string{"-topic", "release", "-surface", "flyout", "-force"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, sketchRoot, "release")
	for _, name := range []string{"baseline-settings.html", "baseline-flyout.html", "mockup.html", "flyout.html", "overlay-settings.md", "overlay-flyout.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s is missing: %v", name, err)
		}
	}
}

func TestSketchRefusesAnExistingTopicWithoutForce(t *testing.T) {
	root := sketchFixtureRoot(t)
	if err := runSketch(root, []string{"-topic", "demo"}); err != nil {
		t.Fatal(err)
	}
	err := runSketch(root, []string{"-topic", "demo"})
	if err == nil || !strings.Contains(err.Error(), "pass -force") {
		t.Fatalf("second run = %v, want a refusal naming -force", err)
	}
}

func TestSketchForceRefreshesTheBaselineAndKeepsTheDelta(t *testing.T) {
	root := sketchFixtureRoot(t)
	if err := runSketch(root, []string{"-topic", "demo"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, sketchRoot, "demo")
	working := filepath.Join(dir, "mockup.html")
	overlay := filepath.Join(dir, "overlay-settings.md")
	if err := os.WriteFile(working, []byte("design delta"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, []byte("kept delta"), 0o644); err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(root, filepath.FromSlash(mockupTemplate))
	if err := os.WriteFile(templatePath, []byte("refreshed template"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSketch(root, []string{"-topic", "demo", "-force"}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "baseline-settings.html")); got != "refreshed template" {
		t.Fatalf("baseline = %q, want the refreshed template", got)
	}
	if got := readFile(t, working); got != "design delta" {
		t.Fatalf("working copy = %q, the delta was clobbered", got)
	}
	if got := readFile(t, overlay); got != "kept delta" {
		t.Fatalf("overlay = %q, the delta doc was clobbered", got)
	}
}

func TestSketchRejectsAPathAsTheTopic(t *testing.T) {
	root := sketchFixtureRoot(t)
	for _, topic := range []string{"../escape", "a/b", `a\b`, ".", ".."} {
		if err := runSketch(root, []string{"-topic", topic}); err == nil {
			t.Fatalf("topic %q was accepted", topic)
		}
	}
}
