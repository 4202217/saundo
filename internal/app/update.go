package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/mod/semver"

	"saundo/cmd/saundo/locales"
	"saundo/internal/settings"
)

// updatePhase is what the update surfaces show.
type updatePhase uint8

const (
	updateIdle updatePhase = iota
	updateChecking
	updateAvailable
	updateDownloading
	updateReady
	updateFailed
)

// updateState is the one update state the A window, the settings card, and
// the flyout chip read. It changes on the main thread only.
type updateState struct {
	phase    updatePhase
	version  string
	notes    string
	date     time.Time
	up       *mygo.Update
	got      int64
	total    int64
	err      string
	disabled bool
	checked  bool
}

// updateEngine is the part of mygo.Updater the update service uses, so a
// test drives the states without a feed.
type updateEngine interface {
	Enabled() bool
	Check(ctx context.Context) (*mygo.Update, error)
	Install(ctx context.Context, up *mygo.Update, progress func(got, total int64)) error
	Relaunch()
	SetPrereleases(on bool)
}

type mygoUpdates struct{}

func (mygoUpdates) Enabled() bool {
	// The darwin build is ad-hoc signed and the release feed carries
	// Windows assets, so an update here would install a Windows binary.
	// Re-enable with a signed, notarized mac feed.
	return updatesSupported && mygo.Updater.Enabled()
}

func (mygoUpdates) Check(ctx context.Context) (*mygo.Update, error) {
	return mygo.Updater.Check(ctx)
}

func (mygoUpdates) Install(ctx context.Context, up *mygo.Update, progress func(got, total int64)) error {
	return up.Install(ctx, progress)
}

func (mygoUpdates) Relaunch() { mygo.App.Relaunch() }

func (mygoUpdates) SetPrereleases(on bool) { mygo.Updater.SetPrereleases(on) }

// saveSettings writes the app-owned preferences over the stored file; a test
// without a store keeps them in memory only. A failure is logged once so a
// full disk or a read-only config directory leaves a trace without repeating
// the line on every toggle.
func (a *App) saveSettings() {
	if a.sets == nil {
		return
	}
	err := a.sets.Update(func(stored *settings.Settings) {
		settings.MergeApp(stored, &a.cfg)
	})
	if err == nil {
		a.settingsErrorLogged = ""
		return
	}
	if a.settingsErrorLogged != err.Error() {
		a.log.Printf("settings save failed: %v", err)
		a.settingsErrorLogged = err.Error()
	}
}

// updateSkipped reports whether the available version is the skipped one.
func (a *App) updateSkipped() bool {
	return a.upd.version != "" && a.upd.version == a.cfg.SkippedVersion
}

// updateChipVisible reports whether the flyout footer shows the update chip:
// an update is waiting, downloading, or ready to restart, and was not
// skipped.
func (a *App) updateChipVisible() bool {
	switch a.upd.phase {
	case updateAvailable, updateDownloading, updateReady:
		return !a.updateSkipped()
	}
	return false
}

// updateChipLabel is the flyout chip's text for the current phase.
func (a *App) updateChipLabel() string {
	switch a.upd.phase {
	case updateDownloading:
		return locales.FlyoutUpdateDownloading(int(a.updateProgress() * 100))
	case updateReady:
		return locales.UpdateReadyShort()
	}
	return locales.FlyoutUpdateAvailable(a.upd.version)
}

// checkForUpdates checks the feed on the main thread, then finishes on it
// through updateUI. The result always shows in the settings version row and
// the flyout chip; no window opens.
func (a *App) checkForUpdates(manual bool) {
	if !manual && !a.cfg.AutoUpdate {
		return
	}
	if a.upd.phase == updateChecking || a.upd.phase == updateDownloading {
		return
	}
	if a.updates == nil || !a.updates.Enabled() {
		a.upd.phase = updateFailed
		a.upd.disabled = true
		a.upd.err = ""
		a.upd.checked = false
		a.refreshUpdateUI()
		return
	}
	a.upd.phase = updateChecking
	a.upd.disabled = false
	a.upd.err = ""
	a.upd.checked = false
	a.refreshUpdateUI()
	a.runAsync(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		up, err := a.updates.Check(ctx)
		a.updateUI(func() { a.finishCheck(manual, up, err) })
	})
}

// finishCheck applies a check's result. A skipped version stays quiet in the
// background but shows when the user asked.
func (a *App) finishCheck(manual bool, up *mygo.Update, err error) {
	a.cfg.LastCheck = time.Now().Format(time.RFC3339)
	a.saveSettings()
	a.upd.checked = err == nil && up == nil
	switch {
	case err != nil:
		a.upd.phase = updateFailed
		a.upd.disabled = false
		a.upd.err = err.Error()
		a.log.Printf("update: check failed: %v", err)
	case up == nil:
		a.upd.phase = updateIdle
		a.upd.version, a.upd.notes, a.upd.up = "", "", nil
		a.upd.err = ""
		a.log.Printf("update: up to date")
	default:
		a.log.Printf("update: %s available", up.Version)
		a.upd.phase = updateAvailable
		a.upd.version, a.upd.notes, a.upd.date, a.upd.up = up.Version, up.Notes, up.Date, up
		a.upd.err = ""
		a.upd.disabled = false
		if up.Version == a.cfg.SkippedVersion && !manual {
			a.upd.phase = updateIdle
			a.upd.version, a.upd.notes, a.upd.up = "", "", nil
		}
	}
	a.rebuildTray()
	a.refreshUpdateUI()
	if err != nil || up == nil || a.upd.phase != updateAvailable {
		return
	}
	// Fetch the localized changelog now, so the notes dialog opens in the
	// app language instead of flashing the manifest's English notes.
	a.loadReleaseHistory()
	if a.cfg.AutoDownload && !a.updateSkipped() {
		a.installUpdate()
	}
}

// installUpdate downloads and installs the available version on a goroutine.
func (a *App) installUpdate() {
	if a.upd.up == nil || a.upd.phase == updateDownloading {
		return
	}
	up := a.upd.up
	ctx, cancel := context.WithCancel(context.Background())
	a.updCancel = cancel
	a.upd.phase = updateDownloading
	a.upd.got, a.upd.total = 0, 0
	a.upd.err = ""
	a.refreshUpdateUI()
	lastPercent := -1
	a.runAsync(func() {
		err := a.updates.Install(ctx, up, func(got, total int64) {
			a.updateUI(func() {
				a.upd.got, a.upd.total = got, total
				pct := int(a.updateProgress() * 100)
				if pct != lastPercent {
					lastPercent = pct
					a.refreshUpdateUI()
				}
			})
		})
		a.updateUI(func() {
			a.updCancel = nil
			if err != nil {
				if ctx.Err() != nil {
					a.upd.phase = updateAvailable
					a.log.Printf("update: download cancelled")
				} else {
					a.upd.phase = updateFailed
					a.upd.err = locales.UpdateInstallFailed()
					a.log.Printf("update: install failed: %v", err)
				}
			} else {
				a.upd.phase = updateReady
				a.log.Printf("update: %s installed, restart to apply", up.Version)
			}
			a.rebuildTray()
			a.refreshUpdateUI()
		})
	})
}

// cancelInstall stops a download and leaves the version available.
func (a *App) cancelInstall() {
	if a.updCancel != nil {
		a.updCancel()
	}
}

// skipUpdate stops offering this version in the background and the flyout.
func (a *App) skipUpdate() {
	if a.upd.version == "" {
		return
	}
	a.cfg.SkippedVersion = a.upd.version
	a.saveSettings()
	a.rebuildTray()
	a.refreshUpdateUI()
}

// relaunchIntoUpdate restarts the app into the installed version.
func (a *App) relaunchIntoUpdate() {
	if a.upd.phase != updateReady {
		return
	}
	a.updates.Relaunch()
}

// refreshUpdateUI redraws the windows and relabels the tray menu.
func (a *App) refreshUpdateUI() {
	a.rebuildTray()
	a.updateUI(func() {})
}

// openUpdateCheck opens the settings update page and checks there, so the
// result stays in the page instead of a window.
func (a *App) openUpdateCheck() {
	a.page = pageUpdate
	a.showSettings()
	a.checkForUpdates(true)
}

// checkForUpdatesInBackground runs a scheduled check on the main thread.
func (a *App) checkForUpdatesInBackground() {
	a.updateUI(func() { a.checkForUpdates(false) })
}

// updateProgress is the download's share, 0 to 1, or 0 without a total.
func (a *App) updateProgress() float64 {
	if a.upd.total <= 0 {
		return 0
	}
	return float64(a.upd.got) / float64(a.upd.total)
}

// updateProgressBar draws the download bar: a hairline track and an ink fill.
func updateProgressBar(c *ui.Context, p palette, frac float32) {
	frac = min(max(frac, 0), 1)
	ui.Box(c).Height(4).Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(r, p.line, 2)
		if frac > 0 {
			pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * frac, H: r.H}, p.fg, 2)
		}
	})
}

// setAutoDownload saves the automatic-download preference.
func (a *App) setAutoDownload(on bool) {
	a.autoDownload = on
	a.cfg.AutoDownload = on
	a.saveSettings()
}

// buildVersion is the version this build reports, or "" when the build
// carries no semantic stamp and the packaged version applies. A local build
// carries git describe, and a commit past a release sorts below it.
func buildVersion() string {
	v := strings.TrimPrefix(Version, "v")
	if !semver.IsValid(normalizeVersion(v)) {
		return ""
	}
	return v
}

// currentVersion is the packaged version the updater compares with.
func (a *App) currentVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return Version
}

func updateDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// updateBytes formats the download's progress in the largest unit that
// keeps it above one.
func updateBytes(got, total int64) string {
	if total <= 0 {
		return ""
	}
	return fmt.Sprintf("%s / %s", byteSize(got), byteSize(total))
}

func byteSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// runAsync runs a job off the UI thread.
func (a *App) runAsync(fn func()) {
	if a.async != nil {
		a.async(fn)
		return
	}
	go fn()
}

// startUpdateChecks checks once shortly after launch and then daily, as the
// automatic-check preference allows.
func (a *App) startUpdateChecks() {
	if a.updates == nil || !a.updates.Enabled() {
		return
	}
	a.updates.SetPrereleases(a.cfg.Beta)
	go func() {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-a.quit:
			return
		}
		a.checkForUpdatesInBackground()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.checkForUpdatesInBackground()
			case <-a.quit:
				return
			}
		}
	}()
}
