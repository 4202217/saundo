package app

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
)

const (
	aboutRepoURL     = "https://github.com/" + updateRepo
	aboutReleasesURL = aboutRepoURL + "/releases"
	aboutIssuesURL   = aboutRepoURL + "/issues"
	aboutLicenseURL  = aboutRepoURL + "/blob/main/LICENSE"
	aboutNoticesURL  = aboutRepoURL + "/blob/main/NOTICE"
)

// aboutPage is the app's identity, its project links, and its license.
func (a *App) aboutPage(c *ui.Context, p palette) {
	card := ui.Column(c).Margin(12, 0, 0, 0).Radius(12).
		Background(p.card).Border(1, p.line).Clip()
	card.Children(func() {
		ui.Row(c).Padding(12).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(56, 56).Radius(14).Shrink(0).Center().Background(p.fg).Children(func() {
				ui.Icon(c, iconMark).Size(35, 35).TextColor(p.bg)
			})
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, locales.SettingsAboutBrand()).FontSize(15).FontWeight(650).SingleLine()
				ui.Text(c, aboutVersionText(a.currentVersion(), a.channelLabel())).
					FontSize(11.5).TextColor(p.muted).FontFeatures("tnum").SingleLine()
			})
		})
	})
	a.aboutCards(c, p)
}

func (a *App) aboutCards(c *ui.Context, p palette) {
	pageCard(c, p, func() {
		aboutLinkRow(c, p, locales.AboutHome(), locales.AboutHomeDesc(), aboutRepoURL)
		rowLine(c, p)
		aboutLinkRow(c, p, locales.AboutReleases(), locales.AboutReleasesDesc(), aboutReleasesURL)
		rowLine(c, p)
		aboutLinkRow(c, p, locales.AboutIssues(), locales.AboutIssuesDesc(), aboutIssuesURL)
	})

	pageCard(c, p, func() {
		aboutRow(c, p, locales.LogTitle(), locales.AboutLogDesc(), iconChevronRight, func() {
			a.showLog = true
			a.logEntered = false
		})
	})

	pageCard(c, p, func() {
		aboutLinkRow(c, p, locales.AboutLicense(), locales.AboutLicenseDesc(), aboutLicenseURL)
		rowLine(c, p)
		aboutLinkRow(c, p, locales.AboutNotices(), locales.AboutNoticesDesc(), aboutNoticesURL)
	})
}

// currentVersionLabel is the version the update surfaces name. It keeps the
// release the build is and drops the git describe stamp, so a local build
// never shows a developer version in a settings row.
func (a *App) currentVersionLabel() string {
	v, local := displayVersion(a.currentVersion())
	switch {
	case v == "":
		return locales.AboutVersionDev()
	case local:
		return v + " · " + locales.VersionLocalTag()
	}
	return v
}

// aboutVersionText names the release the build is, not the git hash it was
// cut from. A local build keeps its release number and says so, so it never
// reads as the published release.
func aboutVersionText(version, channel string) string {
	v, local := displayVersion(version)
	switch {
	case v == "":
		return locales.AboutVersionDev()
	case local:
		return locales.AboutVersionLocal(v)
	}
	return locales.AboutVersion(v, channel)
}

func displayVersion(v string) (string, bool) {
	v = strings.TrimPrefix(v, "v")
	if v == "" || v == "dev" {
		return "", true
	}
	base, rest, ok := strings.Cut(v, "-")
	if !ok || !isBuildStamp(rest) {
		return v, false
	}
	return base, true
}

// isBuildStamp reports whether rest is git describe's "-5-g1e8e94c[-dirty]".
// A prerelease suffix such as "-beta.1" is part of the version, not a stamp.
func isBuildStamp(rest string) bool {
	rest = strings.TrimSuffix(rest, "-dirty")
	count, hash, ok := strings.Cut(rest, "-g")
	if !ok || hash == "" {
		return false
	}
	if _, err := strconv.Atoi(count); err != nil {
		return false
	}
	for _, r := range hash {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// aboutLinkRow is a settings row that opens a page in the browser. The whole
// row is the target, so the external-link glyph appears only where it works.
func aboutLinkRow(c *ui.Context, p palette, title, sub, url string) {
	aboutRow(c, p, title, sub, iconExternal, func() { c.OpenURL(url) })
}

// aboutRow is a settings row whose whole surface performs one action. The
// glyph says where the action lands: another app or another view.
func aboutRow(c *ui.Context, p palette, title, sub string, glyph *ui.SVG, onClick func()) {
	row := ui.ButtonBase(c).MinHeight(50).Padding(7, 14).Gap(12).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(title)
	if row.Hovered() {
		row.Background(p.pill)
	}
	row.OnClick(onClick)
	row.Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(500).SingleLine()
			ui.Text(c, sub).FontSize(11.5).TextColor(p.muted).SingleLine().Ellipsis("…")
		})
		ui.Icon(c, glyph).Size(13, 13).TextColor(p.faint)
	})
}
