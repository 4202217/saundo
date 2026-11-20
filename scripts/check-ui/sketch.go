// check-ui sketch starts a design round from the approved template baseline.
// The baseline is a copy of the current template, which the token gate
// already holds to the app. The working copy starts identical, and the design
// delta is the diff between the two. The release refreshes the templates, so
// a refreshed baseline replaces the old one with -force and leaves the
// working copy and overlay in place.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sketchRoot = ".audit/mockups"

type sketchSurface struct{ template, baseline, working, overlay string }

var sketchSurfaces = map[string]sketchSurface{
	"settings": {mockupTemplate, "baseline-settings.html", "mockup.html", "overlay-settings.md"},
	"flyout":   {flyoutTemplate, "baseline-flyout.html", "flyout.html", "overlay-flyout.md"},
}

func runSketch(root string, args []string) error {
	fs := flag.NewFlagSet("sketch", flag.ContinueOnError)
	topic := fs.String("topic", "", "design topic; the folder name under .audit/mockups")
	surface := fs.String("surface", "settings", "settings or flyout")
	force := fs.Bool("force", false, "refresh the baseline and keep the working copy and overlay")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *topic == "" {
		return fmt.Errorf("sketch: -topic is required")
	}
	if *topic == "." || *topic == ".." || filepath.Base(*topic) != *topic || strings.ContainsAny(*topic, `/\`) {
		return fmt.Errorf("sketch: %q is not a folder name", *topic)
	}
	sc, ok := sketchSurfaces[*surface]
	if !ok {
		return fmt.Errorf("sketch: -surface %q is not settings or flyout", *surface)
	}
	template, err := os.ReadFile(filepath.Join(root, sc.template))
	if err != nil {
		return err
	}

	relDir := sketchRoot + "/" + *topic
	dir := filepath.Join(root, sketchRoot, *topic)
	baseline := filepath.Join(dir, sc.baseline)
	working := filepath.Join(dir, sc.working)
	overlay := filepath.Join(dir, sc.overlay)
	if !*force {
		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("sketch: %s already exists; pass -force to add the surface or refresh the baseline", relDir)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(baseline, template, 0o644); err != nil {
		return err
	}
	if err := writeIfMissing(working, template); err != nil {
		return err
	}
	if err := writeIfMissing(overlay, []byte(overlaySkeleton(*topic, *surface, sc.baseline, sc.working))); err != nil {
		return err
	}
	fmt.Printf("check-ui: %s/%s <- %s\n", relDir, sc.baseline, sc.template)
	fmt.Printf("check-ui: %s/%s is the working copy; edit only the marked blocks\n", relDir, sc.working)
	fmt.Printf("check-ui: %s/%s holds the design delta\n", relDir, sc.overlay)
	fmt.Printf("check-ui: diff the baseline against the working copy: git diff --no-index %s/%s %s/%s\n", relDir, sc.baseline, relDir, sc.working)
	return nil
}

func writeIfMissing(path string, data []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func overlaySkeleton(topic, surface, baseline, working string) string {
	return fmt.Sprintf(`# %s design overlay

Baseline: %s, copied from %s.
Working copy: %s. It starts as the baseline; edit only the marked blocks.
The delta is the diff between the two files. It holds new states, copy, and
layout only; tokens and shell geometry stay in the baseline.

## Scenario IDs

- proposed ID:

## States

- target:
- normal:
- edge:

## Copy

| Key | zh-CN | en |
|---|---|---|
| | | |

## Layout

-

## Rejected

-

## Evidence

- render paths:
`, topic, baseline, sketchSurfaces[surface].template, working)
}
