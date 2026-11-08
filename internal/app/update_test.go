package app

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"saundo/internal/settings"
)

// fakeUpdates drives the update service without a feed.
type fakeUpdates struct {
	enabled    bool
	up         *mygo.Update
	checkErr   error
	installErr error
	installed  bool
	relaunched bool
	pre        bool
}

func (f *fakeUpdates) Enabled() bool { return f.enabled }

func (f *fakeUpdates) Check(context.Context) (*mygo.Update, error) { return f.up, f.checkErr }

func (f *fakeUpdates) Install(_ context.Context, _ *mygo.Update, progress func(got, total int64)) error {
	f.installed = true
	if progress != nil {
		progress(50, 100)
	}
	return f.installErr
}

func (f *fakeUpdates) Relaunch()              { f.relaunched = true }
func (f *fakeUpdates) SetPrereleases(on bool) { f.pre = on }

func updateApp(f *fakeUpdates) *App {
	a := previewApp()
	a.updates = f
	a.async = func(fn func()) { fn() }
	a.cfg = settings.Default()
	a.cfg.AutoUpdate = true
	a.autoUpdate = true
	return a
}

func release() *mygo.Update {
	return &mygo.Update{Version: "0.5.0", Notes: "Fixed things.", Date: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}
}

// The fetch starts when the update is found, not when the dialog opens.
func TestFoundUpdatePrefetchesTheLocalizedNotes(t *testing.T) {
	f := &fakeUpdates{enabled: true, up: release()}
	a := updateApp(f)
	a.async = func(func()) {}
	a.finishCheck(true, release(), nil)
	if a.upd.phase != updateAvailable {
		t.Fatalf("phase = %d, want available", a.upd.phase)
	}
	if a.historyState != historyLoading {
		t.Fatalf("history state = %d, want loading", a.historyState)
	}
}

func TestCheckFindsAnUpdate(t *testing.T) {
	f := &fakeUpdates{enabled: true, up: release()}
	a := updateApp(f)
	a.checkForUpdates(true)
	if a.upd.phase != updateAvailable {
		t.Fatalf("phase = %d, want available", a.upd.phase)
	}
	if a.upd.version != "0.5.0" || a.upd.up == nil {
		t.Fatalf("version = %q up = %v", a.upd.version, a.upd.up)
	}
	if !a.updateChipVisible() {
		t.Fatal("the flyout chip should show an available update")
	}
}

func TestBackgroundCheckHonoursPreferences(t *testing.T) {
	f := &fakeUpdates{enabled: true, up: release()}
	a := updateApp(f)
	a.cfg.AutoUpdate = false
	a.checkForUpdates(false)
	if f.installed || a.upd.phase != updateIdle {
		t.Fatalf("phase = %d with automatic checks off", a.upd.phase)
	}

	a.cfg.SkippedVersion = "0.5.0"
	a.checkForUpdates(false)
	if a.upd.phase != updateIdle {
		t.Fatalf("a skipped version came back in the background: phase %d", a.upd.phase)
	}
	a.checkForUpdates(true)
	if a.upd.phase != updateAvailable {
		t.Fatalf("a manual check should offer the skipped version: phase %d", a.upd.phase)
	}
	if a.updateChipVisible() {
		t.Fatal("a skipped version should stay out of the flyout chip")
	}
}

func TestInstallReachesReady(t *testing.T) {
	f := &fakeUpdates{enabled: true, up: release()}
	a := updateApp(f)
	a.checkForUpdates(true)
	a.installUpdate()
	if !f.installed {
		t.Fatal("install was not called")
	}
	if a.upd.phase != updateReady {
		t.Fatalf("phase = %d, want ready", a.upd.phase)
	}
	if a.upd.got != 50 || a.upd.total != 100 {
		t.Fatalf("progress = %d/%d", a.upd.got, a.upd.total)
	}
	if got := a.updateProgress(); got != 0.5 {
		t.Fatalf("updateProgress = %v", got)
	}
	a.relaunchIntoUpdate()
	if !f.relaunched {
		t.Fatal("relaunch was not called")
	}
}

func TestInstallFailure(t *testing.T) {
	f := &fakeUpdates{enabled: true, up: release(), installErr: errors.New("signature mismatch")}
	a := updateApp(f)
	a.checkForUpdates(true)
	a.installUpdate()
	if a.upd.phase != updateFailed || a.upd.err != "signature mismatch" {
		t.Fatalf("phase = %d err = %q", a.upd.phase, a.upd.err)
	}
	if !a.updateCardVisible() {
		t.Fatal("a failed download should keep the settings card")
	}
}

func TestAutoDownloadSkipsThePrompt(t *testing.T) {
	f := &fakeUpdates{enabled: true, up: release()}
	a := updateApp(f)
	a.cfg.AutoDownload = true
	a.autoDownload = true
	a.checkForUpdates(true)
	if !f.installed {
		t.Fatal("automatic download did not start the install")
	}
	if a.upd.phase != updateReady {
		t.Fatalf("phase = %d, want ready", a.upd.phase)
	}
}

func TestCheckFailureAndDisabledBuild(t *testing.T) {
	f := &fakeUpdates{enabled: true, checkErr: errors.New("offline")}
	a := updateApp(f)
	a.checkForUpdates(true)
	if a.upd.phase != updateFailed || a.upd.err != "offline" {
		t.Fatalf("phase = %d err = %q", a.upd.phase, a.upd.err)
	}

	f2 := &fakeUpdates{enabled: false}
	b := updateApp(f2)
	b.checkForUpdates(true)
	if b.upd.phase != updateFailed || !b.upd.disabled {
		t.Fatalf("a disabled updater: phase = %d disabled = %v", b.upd.phase, b.upd.disabled)
	}
}

func TestManualCheckStaysInline(t *testing.T) {
	f := &fakeUpdates{enabled: true}
	a := updateApp(f)
	a.checkForUpdates(true)
	if a.upd.phase != updateIdle || !a.upd.checked {
		t.Fatalf("up to date: phase = %d checked = %v", a.upd.phase, a.upd.checked)
	}

	f2 := &fakeUpdates{enabled: true, checkErr: errors.New("offline")}
	b := updateApp(f2)
	b.checkForUpdates(true)
	if b.upd.phase != updateFailed || b.upd.checked {
		t.Fatalf("failed check: phase = %d checked = %v", b.upd.phase, b.upd.checked)
	}
}

func TestSkipPersists(t *testing.T) {
	f := &fakeUpdates{enabled: true, up: release()}
	a := updateApp(f)
	a.checkForUpdates(true)
	a.skipUpdate()
	if a.cfg.SkippedVersion != "0.5.0" {
		t.Fatalf("skipped = %q", a.cfg.SkippedVersion)
	}
	if a.updateChipVisible() {
		t.Fatal("the chip should hide after skipping")
	}
	if !a.updateCardVisible() {
		t.Fatal("settings should keep the install entry after skipping")
	}
}

func TestAcceptingAPrereleaseSetsTheEngine(t *testing.T) {
	f := &fakeUpdates{enabled: true}
	a := updateApp(f)
	a.setBeta(true)
	if !f.pre {
		t.Fatal("the engine was not switched to prereleases")
	}
}

func TestBuildVersionStamp(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	cases := map[string]string{
		"v0.4.2":                 "0.4.2",
		"0.4.2-2-gab1c020-dirty": "0.4.2-2-gab1c020-dirty",
		"dev":                    "",
		"":                       "",
	}
	for stamp, want := range cases {
		Version = stamp
		if got := buildVersion(); got != want {
			t.Errorf("buildVersion(%q) = %q, want %q", stamp, got, want)
		}
	}
}

// TestUpdatesDisabledOnDarwin pins the platform gate: the mac build has no
// feed of its own, so Enabled must read false there whatever the updater
// state is. On darwin mygo.Updater is unconfigured in tests anyway, so the
// assertion holds for either mygo answer.
func TestUpdatesDisabledOnDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the gate only bites on darwin")
	}
	if (mygoUpdates{}).Enabled() {
		t.Fatal("updates enabled on darwin; a windows-only feed would offer this build a windows binary")
	}
}
