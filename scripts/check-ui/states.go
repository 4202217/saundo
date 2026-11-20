// check-ui states pins the flyout sketch template's reachable states to the
// state catalog. The token gate pins palette, metrics, and the height rule,
// but it cannot see shape: the catalog grew past the six states the template
// could draw and every gate stayed green, so a sketch round started from a
// baseline that could not sketch connecting, taken-over, the unknown media
// session, the lamp rows, the latency spinner, or the update chip. The reach
// table names the sketch URL that draws each catalog scenario; the gap table
// names what the sketch cannot draw and why. A catalog scenario missing from
// both tables fails, a row naming a state or URL value the template no
// longer carries fails, and a row the catalog no longer declares fails.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const uiCatalogFlyoutGo = "internal/app/ui_catalog_flyout.go"

// sketchReach is one catalog scenario's way into the sketch: the template
// state plus the URL params that draw it.
type sketchReach struct {
	state  string
	params string
}

// flyoutReach maps every catalog flyout scenario to the sketch URL that
// draws it. Each row was read off the scenario's catalog render, not its
// name: the fresh fixture streams without a media session, so states built
// on it carry the streaming footer, the latency row, and the no-media lamp
// even when their names say nothing of it.
var flyoutReach = map[string]sketchReach{
	"flyout.stream":             {state: "streaming", params: "latency=1"},
	"flyout.stream.largeText":   {state: "streaming", params: "latency=1&zoom=1.25"},
	"flyout.nowPlaying.unknown": {state: "streaming", params: "latency=1&session=unknown"},
	"flyout.idle":               {state: "idle", params: "latency=1"},
	"flyout.connecting":         {state: "connecting", params: "latency=1"},
	"flyout.error":              {state: "error", params: "latency=1"},
	"flyout.credential":         {state: "credential", params: "latency=1"},
	"flyout.takenOver":          {state: "taken", params: "latency=1"},
	"flyout.customLatency":      {state: "streaming", params: "latency=1&mode=custom&media=0&lamp=nomedia"},
	"flyout.latencySwitchOff":   {state: "streaming", params: "latency=0&media=0&lamp=nomedia"},
	"flyout.latencyApplying":    {state: "streaming", params: "latency=1&applying=1&media=0&lamp=nomedia"},
	"flyout.empty":              {state: "empty", params: "latency=1"},
	"flyout.coverNone":          {state: "streaming", params: "latency=1&cover=none"},
	"flyout.coverPlaceholder":   {state: "streaming", params: "latency=1&cover=placeholder"},
	"flyout.mediaOff":           {state: "streaming", params: "latency=1&media=0&lamp=nomedia"},
	"flyout.lamp.standby":       {state: "standby", params: "latency=1"},
	"flyout.lamp.collapsed":     {state: "streaming", params: "latency=1&media=0&lamp=nomedia"},
	"flyout.lamp.expanded":      {state: "streaming", params: "latency=1&media=0&lamp=expanded"},
	"flyout.lamp.off":           {state: "streaming", params: "latency=1&media=0&lamp=off"},
	"flyout.lamp.cover":         {state: "streaming", params: "latency=1&media=0&lamp=cover"},
	"flyout.lamp.noMedia":       {state: "streaming", params: "latency=1&media=0&lamp=nomedia"},
	"flyout.update.chip":        {state: "streaming", params: "latency=1&update=1&media=0&lamp=nomedia"},
}

// flyoutGaps names the catalog scenarios the sketch cannot draw, with the
// reason. A gap is a reviewed decision, not an omission.
var flyoutGaps = map[string]string{
	"flyout.noDevice":    "the fixture streams to an empty device card; the sketch couples the empty card to idle",
	"flyout.mac":         "a device-variety sample; the sketch fixes its device list per state",
	"flyout.hover":       "a hover pose; the sketch draws no hover",
	"probe.flyout.enter": "a render probe, not a design state",
}

var (
	catalogIDRe   = regexp.MustCompile(`\{ID: "([^"]+)"`)
	statesBlockRe = regexp.MustCompile(`(?s)const STATES = \[(.*?)\];`)
	stateIDRe     = regexp.MustCompile(`id: '([a-zA-Z]+)'`)
	urlDocRe      = regexp.MustCompile(`(?s)\?state=(.*?)-->`)
	urlParamRe    = regexp.MustCompile(`([a-z]+)=([a-z0-9.|]+)`)
)

// runStates checks the flyout sketch template against the state catalog.
func runStates(root string) error {
	catalog, err := os.ReadFile(filepath.Join(root, uiCatalogFlyoutGo))
	if err != nil {
		return err
	}
	template, err := os.ReadFile(filepath.Join(root, flyoutTemplate))
	if err != nil {
		return err
	}
	problems := checkStates(string(catalog), string(template))
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Println("check-ui states:", p)
		}
		return fmt.Errorf("states: %d drift(s)", len(problems))
	}
	fmt.Printf("check-ui: %d catalog flyout scenarios: %d reachable in %s, %d gapped\n",
		len(flyoutReach)+len(flyoutGaps), len(flyoutReach),
		filepath.Base(flyoutTemplate), len(flyoutGaps))
	fmt.Println("check-ui: states ok")
	return nil
}

// checkStates returns every way the reach and gap tables have parted from
// the catalog and the template.
func checkStates(catalog, template string) []string {
	have := map[string]bool{}
	for _, id := range catalogIDs(catalog) {
		have[id] = true
	}
	states := templateStates(template)
	params := templateParams(template)
	var problems []string
	for _, id := range catalogIDs(catalog) {
		r, ok := flyoutReach[id]
		if !ok {
			if _, gapped := flyoutGaps[id]; gapped {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"%s is in the catalog but neither flyoutReach nor flyoutGaps names it; map the sketch URL that draws it or record the gap", id))
			continue
		}
		if !states[r.state] {
			problems = append(problems, fmt.Sprintf(
				"flyoutReach maps %s to state %q and %s no longer declares it", id, r.state, flyoutTemplate))
		}
		for _, pair := range strings.Split(r.params, "&") {
			key, value, found := strings.Cut(pair, "=")
			if !found {
				continue
			}
			if !params[key][value] {
				problems = append(problems, fmt.Sprintf(
					"flyoutReach maps %s through %s=%s and %s no longer declares it in its URL doc",
					id, key, value, flyoutTemplate))
			}
		}
	}
	var dropped []string
	for id := range flyoutReach {
		if !have[id] {
			dropped = append(dropped, fmt.Sprintf(
				"flyoutReach maps %s and the catalog no longer declares it; drop the row", id))
		}
	}
	for id := range flyoutGaps {
		if !have[id] {
			dropped = append(dropped, fmt.Sprintf(
				"flyoutGaps names %s and the catalog no longer declares it; drop the row", id))
		}
	}
	sort.Strings(dropped)
	problems = append(problems, dropped...)
	return problems
}

// catalogIDs reads the scenario ids one surface's catalog file declares. The
// file grouping is the surface filter; the brace anchors the match to
// Scenario literals, not the device ids inside Apply bodies.
func catalogIDs(catalog string) []string {
	var out []string
	for _, m := range catalogIDRe.FindAllStringSubmatch(catalog, -1) {
		out = append(out, m[1])
	}
	return out
}

// templateStates reads the state ids the sketch's STATES switcher carries.
func templateStates(template string) map[string]bool {
	out := map[string]bool{}
	block := statesBlockRe.FindStringSubmatch(template)
	if block == nil {
		return out
	}
	for _, m := range stateIDRe.FindAllStringSubmatch(block[1], -1) {
		out[m[1]] = true
	}
	return out
}

// templateParams reads the URL interface the template's header declares:
// one ?state= doc listing every key's allowed values.
func templateParams(template string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	doc := urlDocRe.FindStringSubmatch(template)
	if doc == nil {
		return out
	}
	for _, m := range urlParamRe.FindAllStringSubmatch(doc[1], -1) {
		if out[m[1]] == nil {
			out[m[1]] = map[string]bool{}
		}
		for _, v := range strings.Split(m[2], "|") {
			out[m[1]][v] = true
		}
	}
	return out
}
