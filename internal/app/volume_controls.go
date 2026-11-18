package app

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
