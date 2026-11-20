package app

// seedSelection starts the panel's selection on the last session's first
// target; syncSelection falls back to the first listed device before any
// session.
func (a *App) seedSelection(last []string) {
	if len(last) > 0 {
		a.selID = last[0]
	}
}

// syncSelection keeps the flyout's selected row on a device that is still in
// the list, falling back to the first row.
func (a *App) syncSelection() {
	for _, d := range a.snap.Devices {
		if d.ID == a.selID {
			return
		}
	}
	if len(a.snap.Devices) > 0 {
		a.selID = a.snap.Devices[0].ID
	}
}

// selectDevice moves the flyout selection. The volume follows the device's own
// row now, so the selection only marks the device the tray speaks for.
func (a *App) selectDevice(id string) {
	a.selID = id
}
