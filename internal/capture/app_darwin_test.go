//go:build darwin

package capture

import (
	"os"
	"testing"

	"saundo/internal/darwin"
)

func testApps() []runningApp {
	return []runningApp{
		{PID: 100, Name: "Google Chrome", BundleID: "com.google.Chrome", Path: "/Applications/Google Chrome.app", Policy: appPolicyRegular},
		{PID: 200, Name: "Music", BundleID: "com.apple.Music", Path: "/System/Applications/Music.app", Policy: appPolicyRegular},
		{PID: 300, Name: "magpie Graphics and Media", BundleID: "com.apple.WebKit.GPU", Policy: 1},
		{PID: 101, Name: "Google Chrome Helper", BundleID: "com.google.Chrome.helper", Policy: 2},
		{PID: 900, Name: "saundo", BundleID: "com.saundo.app", Policy: 1},
	}
}

func testProcs() []processObject {
	return []processObject{
		{ID: 10, PID: 100, BundleID: "com.google.Chrome"},
		{ID: 11, PID: 101, BundleID: "com.google.Chrome.helper", RunningOutput: true},
		{ID: 20, PID: 200, BundleID: "com.apple.Music"},
		{ID: 30, PID: 300, BundleID: "com.apple.WebKit.GPU", RunningOutput: true},
		{ID: 90, PID: 900, BundleID: "com.saundo.app", RunningOutput: true},
	}
}

// TestBuildAppListGroupsHelpers pins the picker's contract: an app is listed
// once even when its audio comes from a helper process, and a background
// process with output but no owner gets its own entry.
func TestBuildAppListGroupsHelpers(t *testing.T) {
	got := buildAppList(testApps(), testProcs(), 900)
	want := []AppInfo{
		{PID: 100, Name: "Google Chrome", Path: "/Applications/Google Chrome.app", ID: "com.google.Chrome", Active: true},
		{PID: 300, Name: "magpie Graphics and Media", ID: "pid:300", Active: true},
		{PID: 200, Name: "Music", Path: "/System/Applications/Music.app", ID: "com.apple.Music", Active: false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d apps, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.PID != w.PID || g.Name != w.Name || g.Path != w.Path || g.ID != w.ID || g.Active != w.Active {
			t.Errorf("app %d = %+v, want %+v", i, g, w)
		}
	}
}

// TestBuildAppListSkipsSelf keeps saundo out of its own capture picker; a tap
// on its own output would feed back into the stream.
func TestBuildAppListSkipsSelf(t *testing.T) {
	for _, a := range buildAppList(testApps(), testProcs(), 900) {
		if a.PID == 900 {
			t.Fatalf("self pid 900 is listed: %+v", a)
		}
	}
}

// TestBuildAppListNamesUnattributedProcess covers a rendering process that
// LaunchServices does not know, such as a command-line player.
func TestBuildAppListNamesUnattributedProcess(t *testing.T) {
	got := buildAppList(nil, []processObject{
		{ID: 40, PID: 400, RunningOutput: true},
		{ID: 41, PID: 401, BundleID: "", RunningOutput: true},
	}, 900)
	if len(got) != 2 {
		t.Fatalf("got %d apps, want 2: %+v", len(got), got)
	}
	if got[0].Name != "PID 400" || got[0].ID != "pid:400" || !got[0].Active {
		t.Errorf("unnamed process = %+v, want PID 400 active", got[0])
	}
}

// TestAppsTapIDs pins multi-select resolution: bundle IDs take their app
// family, pid IDs take one process, and a closed app does not hide the rest.
func TestAppsTapIDs(t *testing.T) {
	names, resolved, objects := appsTapIDs(testApps(), testProcs(), []string{
		"com.google.Chrome", "com.apple.Music", "com.closed.App",
	})
	if len(objects) != 3 || objects[0] != 10 || objects[1] != 11 || objects[2] != 20 {
		t.Fatalf("objects = %v, want [10 11 20]", objects)
	}
	if len(names) != 2 || names[0] != "Google Chrome" || names[1] != "Music" {
		t.Fatalf("names = %v", names)
	}
	if len(resolved) != 2 || resolved[0] != "com.google.Chrome" || resolved[1] != "com.apple.Music" {
		t.Fatalf("resolved = %v, want the two open apps", resolved)
	}

	names, resolved, objects = appsTapIDs(testApps(), testProcs(), []string{"pid:300"})
	if len(objects) != 1 || objects[0] != 30 || len(names) != 1 || names[0] != "magpie Graphics and Media" {
		t.Fatalf("pid:300 -> %v %v", names, objects)
	}
	if len(resolved) != 1 || resolved[0] != "pid:300" {
		t.Fatalf("pid:300 resolved = %v", resolved)
	}

	if names, resolved, objects := appsTapIDs(testApps(), testProcs(), []string{"com.closed.App"}); len(objects) != 0 || len(names) != 0 || len(resolved) != 0 {
		t.Fatalf("closed app resolved to %v %v %v", names, resolved, objects)
	}
}

// TestBuildAppListMergesDuplicateBundle keeps a relaunch's leftover process
// from showing the same app twice.
func TestBuildAppListMergesDuplicateBundle(t *testing.T) {
	apps := []runningApp{
		{PID: 100, Name: "Google Chrome", BundleID: "com.google.Chrome", Policy: appPolicyRegular},
		{PID: 102, Name: "Google Chrome", BundleID: "com.google.Chrome", Policy: appPolicyRegular},
	}
	procs := []processObject{
		{ID: 12, PID: 102, BundleID: "com.google.Chrome", RunningOutput: true},
	}
	got := buildAppList(apps, procs, 0)
	if len(got) != 1 {
		t.Fatalf("got %d apps, want 1: %+v", len(got), got)
	}
	if got[0].PID != 100 || got[0].ID != "com.google.Chrome" || !got[0].Active {
		t.Fatalf("family = %+v, want pid 100 active", got[0])
	}
}

// TestAppFamilyFollowsBundlePrefix pins the capture side of the grouping: a
// regular app's tap includes its helpers, while a standalone background
// process is tapped alone.
func TestAppFamilyFollowsBundlePrefix(t *testing.T) {
	apps := testApps()
	procs := testProcs()
	cases := []struct {
		name string
		pid  uint32
		root runningApp
		want []uint32
	}{
		{"chrome with helper", apps[0].PID, apps[0], []uint32{10, 11}},
		{"music", apps[1].PID, apps[1], []uint32{20}},
		{"standalone webkit gpu", apps[2].PID, apps[2], []uint32{30}},
	}
	for _, c := range cases {
		got := appFamily(procs, c.pid, c.root)
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v, want %v", c.name, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			}
		}
	}
}

// TestAppFamilyUnknownRootFallsBackToPID covers a process LaunchServices
// does not list, such as a command-line player: the zero root must not widen
// into every process with an empty bundle.
func TestAppFamilyUnknownRootFallsBackToPID(t *testing.T) {
	got := appFamily(testProcs(), 200, runningApp{})
	if len(got) != 1 || got[0] != 20 {
		t.Fatalf("got %v, want [20]", got)
	}
}

// TestAudioApplicationsReadsLiveLists exercises the AppKit and HAL reads the
// picker depends on. It runs on any macOS host without needing audio capture
// consent.
func TestAudioApplicationsReadsLiveLists(t *testing.T) {
	if !AppCaptureSupported() {
		t.Skip("Core Audio process taps are unavailable")
	}
	apps, err := AudioApplications()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range apps {
		if a.PID == uint32(os.Getpid()) {
			t.Fatalf("saundo is listed as capturable: %+v", a)
		}
	}
}

// TestMixdownTapMuteBehavior pins the per-app tap's local-output behavior at
// creation: muted captures the app without sending it to the speakers.
func TestMixdownTapMuteBehavior(t *testing.T) {
	if err := darwin.EnsureCoreAudio(); err != nil {
		t.Skipf("Core Audio unavailable: %v", err)
	}
	for _, muted := range []bool{false, true} {
		desc, err := newMixdownTapDescription([]uint32{1}, muted)
		if err != nil {
			t.Fatal(err)
		}
		got := darwin.SendInt(desc, "isMuted") != 0
		darwin.Send(desc, "release")
		if got != muted {
			t.Errorf("tap muted = %v, want %v", got, muted)
		}
	}
}

// TestMixdownTapDescriptionBuilds pins the CATapDescription selector and the
// NSNumber boxing. It creates a description only, not a tap, so it needs no
// capture consent.
func TestMixdownTapDescriptionBuilds(t *testing.T) {
	if err := darwin.EnsureCoreAudio(); err != nil {
		t.Skipf("Core Audio unavailable: %v", err)
	}
	desc, err := newMixdownTapDescription([]uint32{1}, false)
	if err != nil {
		t.Fatal(err)
	}
	if desc == 0 {
		t.Fatal("nil tap description")
	}
	darwin.Send(desc, "release")
}

func TestOwnerApp(t *testing.T) {
	regular := map[string]runningApp{
		"com.google.Chrome": {PID: 100, BundleID: "com.google.Chrome"},
		"com.apple.Music":   {PID: 200, BundleID: "com.apple.Music"},
	}
	cases := []struct {
		bundle string
		pid    uint32
		ok     bool
	}{
		{"com.google.Chrome.helper", 100, true},
		{"com.google.Chrome.helper.renderer", 100, true},
		{"com.apple.Music", 200, true},
		{"com.other.App", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := ownerApp(c.bundle, regular)
		if ok != c.ok || (ok && got.PID != c.pid) {
			t.Errorf("ownerApp(%q) = (%+v, %v), want pid %d ok %v", c.bundle, got, ok, c.pid, c.ok)
		}
	}
}
