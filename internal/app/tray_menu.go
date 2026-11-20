package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo"

	"saundo/cmd/saundo/locales"
	"saundo/internal/lamp"
	"saundo/internal/nowplaying"
)

// trayMenuDeviceRows caps the receiver rows so the fixed actions stay near
// the pointer. The rest of the list opens the receivers page.
const trayMenuDeviceRows = 6

// trayMenuModel is everything the tray menu shows, derived from the engine
// snapshot, the media session, the ambient light, and the update state.
// Rebuilding compares the model, so an unchanged model keeps the cached menu.
type trayMenuModel struct {
	locale   string
	dark     bool
	phase    phase
	selected string
	members  []string
	devices  []trayMenuDevice

	media        bool
	mediaAUMID   string
	mediaPlaying bool
	canPlay      bool
	canPause     bool
	canNext      bool
	canPrev      bool
	coverKey     string

	lampOff    bool
	lampCover  bool
	lampPreset string

	update        updatePhase
	updateVersion string
	updatePercent int
}

// trayMenuDevice is one receiver row.
type trayMenuDevice struct {
	id     string
	name   string
	family string
	online bool
}

// trayMenuKey is what the tray menu's shape depends on. The tooltip follows
// more of the snapshot and is compared on its own.
type trayMenuKey struct {
	locale   string
	dark     bool
	phase    phase
	selected string
	members  string
	devices  string

	media        bool
	mediaAUMID   string
	mediaPlaying bool
	canPlay      bool
	canPause     bool
	canNext      bool
	canPrev      bool
	coverKey     string

	lampOff    bool
	lampCover  bool
	lampPreset string

	update        updatePhase
	updateVersion string
	updatePercent int
}

func (a *App) trayModel(locale string, dark bool) trayMenuModel {
	m := trayMenuModel{
		locale:     locale,
		dark:       dark,
		phase:      phaseOf(a.snap),
		selected:   a.snap.SelectedID,
		members:    append([]string(nil), a.snap.ActiveTargets...),
		lampOff:    a.snap.LampOff,
		lampCover:  a.snap.LampCover,
		lampPreset: lamp.Normalize(a.snap.LampPreset),
		devices:    make([]trayMenuDevice, len(a.snap.Devices)),
	}
	if m.selected == "" {
		m.selected = a.selID
	}
	for i, d := range a.snap.Devices {
		m.devices[i] = trayMenuDevice{id: d.ID, name: d.Name, family: stereoIconKey(d, d.IsStereoGroup()), online: d.Online}
	}
	if s, ok := a.nowSession(); ok && a.cfg.PlaybackControls && a.nowHasControls(s) {
		m.media = true
		m.mediaAUMID = s.AUMID
		m.mediaPlaying = s.Playing
		m.canPlay = s.CanPlay
		m.canPause = s.CanPause
		m.canNext = s.CanNext
		m.canPrev = s.CanPrev
		m.coverKey = a.nowArtKey + "|" + strconv.FormatBool(dark)
	}
	if a.updateChipVisible() {
		m.update = a.upd.phase
		m.updateVersion = a.upd.version
		if m.update == updateDownloading {
			m.updatePercent = int(a.updateProgress() * 100)
		}
	}
	return m
}

func (m trayMenuModel) key() trayMenuKey {
	var b strings.Builder
	for _, d := range m.devices {
		fmt.Fprintf(&b, "%d:%s|%d:%s|%s|%t\n", len(d.id), d.id, len(d.name), d.name, d.family, d.online)
	}
	return trayMenuKey{
		locale:        m.locale,
		dark:          m.dark,
		phase:         m.phase,
		selected:      m.selected,
		members:       strings.Join(m.members, ","),
		devices:       b.String(),
		media:         m.media,
		mediaAUMID:    m.mediaAUMID,
		mediaPlaying:  m.mediaPlaying,
		canPlay:       m.canPlay,
		canPause:      m.canPause,
		canNext:       m.canNext,
		canPrev:       m.canPrev,
		coverKey:      m.coverKey,
		lampOff:       m.lampOff,
		lampCover:     m.lampCover,
		lampPreset:    m.lampPreset,
		update:        m.update,
		updateVersion: m.updateVersion,
		updatePercent: m.updatePercent,
	}
}

// deviceIcon is the key of the receiver's glyph: the device in session takes
// the accent-colored variant for the appearance, the other rows the template
// in the menu's text color.
func (m trayMenuModel) deviceIcon(d trayMenuDevice) string {
	if !m.isMember(d.id) {
		return "dev:" + d.family
	}
	if m.dark {
		return "devond:" + d.family
	}
	return "devon:" + d.family
}

// isMember reports whether the target is in the live or wanted session, so
// every member row carries the session glyph and state, not just the primary.
func (m trayMenuModel) isMember(id string) bool {
	for _, member := range m.members {
		if member == id {
			return true
		}
	}
	return false
}

// lampSubtitle names the light's current state without opening the submenu.
func (m trayMenuModel) lampSubtitle() string {
	if m.lampOff {
		return locales.LampOffStatus()
	}
	if m.lampCover {
		return locales.LampPresetCover()
	}
	return lampPresetName(m.lampPreset)
}

func (m trayMenuModel) lampIconKey() string {
	if m.lampOff {
		return "lampOff"
	}
	if m.lampCover {
		return "lamp:cover"
	}
	return "lamp:" + m.lampPreset
}

func (a *App) trayMenuItems(m trayMenuModel) []*mygo.MenuItem {
	items := make([]*mygo.MenuItem, 0, len(m.devices)+16)
	if len(m.devices) == 0 {
		items = append(items, &mygo.MenuItem{
			Label:    locales.FlyoutSearchAgain(),
			Subtitle: locales.ReceiverEmptyTitle(),
			Icon:     a.menuIcons["update"],
			Click:    func(*mygo.MenuItem, *mygo.Window) { a.eng.Refresh() },
		})
	}
	shown := m.devices
	if len(shown) > trayMenuDeviceRows {
		shown = shown[:trayMenuDeviceRows]
	}
	for _, d := range shown {
		active := m.isMember(d.id)
		item := &mygo.MenuItem{
			Label: d.name,
			Icon:  a.menuIcons[m.deviceIcon(d)],
			Click: func(*mygo.MenuItem, *mygo.Window) { a.connectDevice(d.id) },
		}
		if !d.online {
			item.Subtitle = locales.ActionOffline()
		}
		if active {
			switch m.phase {
			case phaseConnecting:
				item.Subtitle = locales.StatusConnecting()
			case phaseStreaming:
				if a.snap.AudioIdle {
					item.Subtitle = locales.SummaryConnected()
				} else {
					item.Subtitle = locales.StatusStreaming()
				}
			case phaseError:
				item.Subtitle = locales.StatusConnectionFailed()
			}
		}
		if m.phase == phaseConnecting || (active && (m.phase == phaseStreaming || m.phase == phaseError)) {
			item.Disabled = true
		}
		items = append(items, item)
	}
	if len(m.devices) > trayMenuDeviceRows {
		items = append(items, &mygo.MenuItem{
			Label: locales.TrayMenuMore(),
			Icon:  a.menuIcons["dev:homepod"],
			Click: func(*mygo.MenuItem, *mygo.Window) {
				a.page = pageReceivers
				a.showSettings()
			},
		})
	}
	if session := a.traySessionItem(m); session != nil {
		items = append(items, session)
	}
	items = append(items, mygo.Separator())
	if media := a.trayMediaItems(m); len(media) > 0 {
		items = append(items, media...)
		items = append(items, mygo.Separator())
	}
	items = append(items, &mygo.MenuItem{
		Label:    locales.FlyoutLampSection(),
		Subtitle: m.lampSubtitle(),
		Icon:     a.menuIcons[m.lampIconKey()],
		Submenu:  a.trayLampItems(m),
	})
	items = append(items, mygo.Separator())
	items = append(items,
		&mygo.MenuItem{Label: locales.TrayShowPanel(), Icon: a.menuIcons["open"], Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		&mygo.MenuItem{Label: locales.TraySettings(), Icon: a.menuIcons["settings"], Click: func(*mygo.MenuItem, *mygo.Window) { a.showSettings() }},
	)
	if update := a.trayUpdateItem(m); update != nil {
		items = append(items, update)
	}
	items = append(items, mygo.Separator())
	items = append(items, &mygo.MenuItem{Role: mygo.RoleQuit, Label: locales.TrayQuit(), Icon: a.menuIcons["quit"]})
	return items
}

// traySessionItem is the one-click session action under the receiver rows.
// The rows themselves only connect or switch.
func (a *App) traySessionItem(m trayMenuModel) *mygo.MenuItem {
	switch m.phase {
	case phaseConnecting:
		return &mygo.MenuItem{Label: locales.TrayMenuCancelConnect(), Icon: a.menuIcons["close"], Click: func(*mygo.MenuItem, *mygo.Window) { a.disconnect() }}
	case phaseStreaming:
		return &mygo.MenuItem{Label: locales.ActionDisconnect(), Icon: a.menuIcons["unlink"], Click: func(*mygo.MenuItem, *mygo.Window) { a.disconnect() }}
	case phaseError:
		return &mygo.MenuItem{
			Label:    locales.TrayMenuRetry(),
			Icon:     a.menuIcons["update"],
			Disabled: m.selected == "",
			Click:    func(*mygo.MenuItem, *mygo.Window) { a.connectDevice(m.selected) },
		}
	}
	return nil
}

// trayMediaItems follows the flyout's transport: previous, play or pause,
// next, with the current track under the play/pause row.
func (a *App) trayMediaItems(m trayMenuModel) []*mygo.MenuItem {
	if !m.media {
		return nil
	}
	control := func(action nowplaying.Action) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			if a.controlNow != nil {
				a.controlNow(m.mediaAUMID, action)
			}
		}
	}
	items := make([]*mygo.MenuItem, 0, 3)
	if m.canPrev {
		items = append(items, &mygo.MenuItem{Label: locales.FlyoutPrevious(), Icon: a.menuIcons["prev"], Click: control(nowplaying.ActionPrevious)})
	}
	var playPause *mygo.MenuItem
	if m.mediaPlaying && m.canPause {
		playPause = &mygo.MenuItem{Label: locales.FlyoutPause(), Icon: a.menuIcons["pause"], Click: control(nowplaying.ActionPause)}
	} else if m.canPlay {
		playPause = &mygo.MenuItem{Label: locales.FlyoutPlay(), Icon: a.menuIcons["play"], Click: control(nowplaying.ActionPlay)}
	}
	if playPause != nil {
		items = append(items, playPause)
	}
	if m.canNext {
		items = append(items, &mygo.MenuItem{Label: locales.FlyoutNext(), Icon: a.menuIcons["next"], Click: control(nowplaying.ActionNext)})
	}
	return items
}

// trayLampItems disables every choice while the light is off and the current
// choice while it is on. The power row turns the light on; the other choices
// switch to them. This matches the flyout and settings.
func (a *App) trayLampItems(m trayMenuModel) []*mygo.MenuItem {
	items := []*mygo.MenuItem{
		{
			Label: m.lampPowerLabel(),
			Icon:  a.menuIcons["quit"],
			Click: func(*mygo.MenuItem, *mygo.Window) { a.setLampOff(!m.lampOff) },
		},
		mygo.Separator(),
		{
			Label:    lampCheck(locales.LampPresetCover(), m.lampCover && !m.lampOff),
			Subtitle: m.lampLastUsed(m.lampCover, ""),
			Icon:     a.menuIcons["lamp:cover"],
			Disabled: m.lampOff || m.lampCover,
			Click:    func(*mygo.MenuItem, *mygo.Window) { a.setLampCover(true) },
		},
	}
	for _, p := range lamp.All() {
		id := p.ID
		items = append(items, &mygo.MenuItem{
			Label:    lampCheck(lampPresetName(id), !m.lampCover && m.lampPreset == id && !m.lampOff),
			Subtitle: m.lampLastUsed(!m.lampCover, id),
			Icon:     a.menuIcons["lamp:"+id],
			Disabled: m.lampOff || (!m.lampCover && m.lampPreset == id),
			Click:    func(*mygo.MenuItem, *mygo.Window) { a.chooseLampPreset(id) },
		})
	}
	return items
}

// lampLastUsed names the stored choice while the light is off. While the
// light is on the current choice is disabled and carries no suffix.
// lampCheck marks the current choice with a single-color check. The check is
// part of the label, not a native check column, so it greys with the
// disabled row and does not disturb the icon column.
func lampCheck(label string, current bool) string {
	if current {
		return label + " ✔"
	}
	return label
}

func (m trayMenuModel) lampLastUsed(match bool, id string) string {
	if !m.lampOff || !match || (id != "" && m.lampPreset != id) {
		return ""
	}
	return locales.TrayMenuLastUsed()
}

func (m trayMenuModel) lampPowerLabel() string {
	if m.lampOff {
		return locales.LampTurnOn()
	}
	return locales.LampTurnOff()
}

func (a *App) trayUpdateItem(m trayMenuModel) *mygo.MenuItem {
	switch m.update {
	case updateAvailable:
		return &mygo.MenuItem{
			Label: locales.TrayMenuDownload(m.updateVersion),
			Icon:  a.menuIcons["update"],
			Click: func(*mygo.MenuItem, *mygo.Window) { a.installUpdate() },
		}
	case updateDownloading:
		return &mygo.MenuItem{
			Label:    locales.FlyoutUpdateDownloading(m.updatePercent),
			Icon:     a.menuIcons["update"],
			Disabled: true,
		}
	case updateReady:
		return &mygo.MenuItem{
			Label: locales.TrayMenuInstall(m.updateVersion),
			Icon:  a.menuIcons["quit"],
			Click: func(*mygo.MenuItem, *mygo.Window) { a.relaunchIntoUpdate() },
		}
	}
	return nil
}

func (a *App) rebuildTray() {
	if a.tray == nil {
		return
	}
	if a.menuIcons == nil {
		a.menuIcons = loadMenuIcons()
	}
	m := a.trayModel(mygo.App.Locale(), mygo.Theme.IsDark())
	a.refreshCoverMenuIcon(m)
	key := m.key()
	if a.trayMenu == nil || key != a.menuKey {
		a.menuKey = key
		a.trayMenu = mygo.NewMenu(a.trayMenuItems(m))
	}
	tip := locales.TrayTooltip()
	if m.phase != phaseIdle {
		tip += " · " + a.statusText()
	}
	if a.updateChipVisible() {
		if a.upd.phase == updateReady {
			tip += " · " + locales.UpdateReadyShort()
		} else {
			tip += " · " + locales.FlyoutUpdateAvailable(a.upd.version)
		}
	}
	if tip != a.trayTip {
		a.trayTip = tip
		a.tray.SetToolTip(tip)
	}
	a.refreshTrayIcon()
}

// refreshCoverMenuIcon keeps the cover choice's round preview in step with
// the artwork; the template previews are static.
func (a *App) refreshCoverMenuIcon(m trayMenuModel) {
	if m.coverKey == a.menuCoverKey {
		return
	}
	a.menuCoverKey = m.coverKey
	colors, weights := coverPreviewColors(a.nowFieldLight, a.nowFieldDark, m.dark)
	if icon := previewMenuIcon(colors, weights); icon != nil {
		a.menuIcons["lamp:cover"] = icon
	}
}
