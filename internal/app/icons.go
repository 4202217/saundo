package app

import "github.com/egoist/mygo/ui"

// The icons are single-colour SVGs: ui.Icon tints them with the text color,
// so one definition serves both appearances.
var (
	iconMark = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="7.4" cy="8.8" r="2.1" fill="currentColor" stroke="none"/><circle cx="16.9" cy="8.8" r="2.3"/><path d="M7.4 14.2c1.5 2.5 3.2 3.8 4.8 3.8s3.3-1.3 4.7-3.8"/></g></svg>`)

	// The state faces: the app's smile changes its mouth to show what it is
	// doing. The tray shows the same faces as its icon.
	faceIdleOpen = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="7.4" cy="8.8" r="2.1" fill="currentColor" stroke="none"/><circle cx="16.9" cy="8.8" r="2.3"/></g><path d="M8.2 15.4h7.8" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`)
	faceStream   = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="7.4" cy="8.8" r="2.1" fill="currentColor" stroke="none"/><circle cx="16.9" cy="8.8" r="2.3"/></g><g fill="currentColor"><rect x="8.1" y="14.5" width="1.3" height="2.6" rx=".65"/><rect x="9.9" y="13.7" width="1.3" height="4.2" rx=".65"/><rect x="11.7" y="14.2" width="1.3" height="3.2" rx=".65"/><rect x="13.5" y="13.4" width="1.3" height="4.8" rx=".65"/><rect x="15.3" y="14.6" width="1.3" height="2.4" rx=".65"/></g></svg>`)
	faceConnect  = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="7.4" cy="8.8" r="2.1" fill="currentColor" stroke="none"/><circle cx="16.9" cy="8.8" r="2.3"/></g><g fill="currentColor"><circle cx="9.6" cy="15.4" r="1.1"/><circle cx="12.15" cy="15.4" r="1.1"/><circle cx="14.7" cy="15.4" r="1.1"/></g></svg>`)
	faceError    = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M5.9 7.3l3.0 3.0M8.9 7.3l-3.0 3.0M15.4 7.3l3.0 3.0M18.4 7.3l-3.0 3.0"/></g><path d="M8.4 16.1l1.7-1.3 1.7 1.3 1.7-1.3 1.7 1.3" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	faceSearch   = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="7.4" cy="8.8" r="2.1" fill="currentColor" stroke="none"/><circle cx="16.9" cy="8.8" r="2.3"/></g><circle cx="12.15" cy="15.4" r="1.9" fill="none" stroke="currentColor" stroke-width="1.8"/></svg>`)

	iconSpeaker = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"><rect x="6.5" y="2.5" width="11" height="19" rx="5.5"/><path d="M9.5 18.5h5"/><circle cx="12" cy="8" r="1" fill="currentColor" stroke="none"/></g></svg>`)

	iconVolume = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5 6.5 8.5H3.5v7h3L11 19z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/><path d="M18.8 6a9 9 0 0 1 0 12"/></g></svg>`)

	iconLink         = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M10 14l4-4M8 11.5 6.2 13.3a3.8 3.8 0 0 0 5.4 5.4l1.8-1.8M16 12.5l1.8-1.8a3.8 3.8 0 0 0-5.4-5.4L10.6 7"/></g></svg>`)
	iconUnlink       = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M10 14l4-4M8 11.5 6.2 13.3a3.8 3.8 0 0 0 5.4 5.4l1.8-1.8M16 12.5l1.8-1.8a3.8 3.8 0 0 0-5.4-5.4L10.6 7"/><path d="M5 5l14 14"/></g></svg>`)
	iconSlash        = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="8"/><path d="M6.5 6.5l11 11"/></g></svg>`)
	iconClose        = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M6.5 6.5l11 11M17.5 6.5l-11 11"/></g></svg>`)
	iconRefresh      = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M19.5 12a7.5 7.5 0 1 1-2.2-5.3M19.5 4v4.5h-4.5"/></g></svg>`)
	iconReset        = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M4.5 12a7.5 7.5 0 1 0 2.2-5.3M4.5 4v4.5h4.5"/></g></svg>`)
	iconDoc          = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round"><rect x="5" y="3" width="14" height="18" rx="2"/><path d="M8 8h8M8 12h8M8 16h5"/></g></svg>`)
	iconGear         = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></g></svg>`)
	iconBell         = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8a6 6 0 1 0-12 0c0 5-2 6-2 6h16s-2-1-2-6"/><path d="M13.7 20a2 2 0 0 1-3.4 0"/></g></svg>`)
	iconBellOff      = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8a6 6 0 1 0-12 0c0 5-2 6-2 6h16s-2-1-2-6"/><path d="M13.7 20a2 2 0 0 1-3.4 0"/><path d="M4 4l16 16"/></g></svg>`)
	iconNote         = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V6l8-2v12"/><circle cx="6.5" cy="18" r="2.5"/><circle cx="14.5" cy="16" r="2.5"/></g></svg>`)
	iconImage        = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="3.5" y="4.5" width="17" height="15" rx="2.5"/><circle cx="9" cy="10" r="1.8"/><path d="M4.5 17l4.8-4.5 3.2 3 2.7-2.4 4.3 4.4"/></g></svg>`)
	iconPower        = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.8 0"/></g></svg>`)
	iconCheck        = iconSVG(`<svg viewBox="0 0 12 12" fill="none"><path d="M2 6.2 4.8 9 10 3.2" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	iconChevron      = iconSVG(`<svg viewBox="0 0 12 12" fill="none"><path d="M2.5 4.5 6 8l3.5-3.5" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	iconChevronRight = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><path d="M9 5l7 7-7 7" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`)
	iconPrev         = iconSVG(`<svg viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="5" width="2.4" height="14" rx="1"/><path d="M18.6 5.4a1 1 0 0 1 0 1.2L11.4 12l7.2 5.4a1 1 0 0 1-1.2 1.6l-8-6a1 1 0 0 1 0-1.6l8-6a1 1 0 0 1 1.2 0z"/></svg>`)
	iconPlay         = iconSVG(`<svg viewBox="0 0 24 24" fill="currentColor"><path d="M8 5.5a1 1 0 0 1 1.5-.9l10 6.5a1 1 0 0 1 0 1.8l-10 6.5A1 1 0 0 1 8 18.5z"/></svg>`)
	iconPause        = iconSVG(`<svg viewBox="0 0 24 24" fill="currentColor"><rect x="7" y="5" width="3.5" height="14" rx="1.2"/><rect x="13.5" y="5" width="3.5" height="14" rx="1.2"/></svg>`)
	iconNext         = iconSVG(`<svg viewBox="0 0 24 24" fill="currentColor"><rect x="15.6" y="5" width="2.4" height="14" rx="1"/><path d="M5.4 5.4a1 1 0 0 0 0 1.2L12.6 12l-7.2 5.4a1 1 0 0 0 1.2 1.6l8-6a1 1 0 0 0 0-1.6l-8-6a1 1 0 0 0-1.2 0z"/></svg>`)
	iconDownload     = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v11M7.5 9.5 12 14l4.5-4.5M5 19h14"/></g></svg>`)
	iconPlus         = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></g></svg>`)
	iconPencil       = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 20h4L19.5 8.5a2.1 2.1 0 0 0-3-3L5 17v3z"/><path d="M14.5 5.5l4 4"/></g></svg>`)
	iconKey          = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="8.5" cy="15.5" r="3.5"/><path d="M11 13 20 4M17 7l2.5 2.5M14.5 9.5l2 2"/></g></svg>`)
	iconInfo         = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"><circle cx="12" cy="12" r="8.5"/><path d="M12 11.2v5"/><circle cx="12" cy="8" r="1" fill="currentColor" stroke="none"/></g></svg>`)
	iconExternal     = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M14 4h6v6M20 4l-8.5 8.5"/><path d="M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4"/></g></svg>`)

	iconSliders = iconSVG(`<svg viewBox="0 0 20 20" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M3 6h14M3 10h14M3 14h14"/></g><g fill="currentColor"><circle cx="7" cy="6" r="2.1"/><circle cx="13" cy="10" r="2.1"/><circle cx="8" cy="14" r="2.1"/></g></svg>`)
	iconWave    = iconSVG(`<svg viewBox="0 0 20 20" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M4 8v4M8 5v10M12 8v4M16 6v8"/></g></svg>`)
	iconLamp    = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M7.2 3.5h9.6l-1.7 6.4H8.9L7.2 3.5Z"/><path d="M12 9.9v7.6"/><path d="M8 17.5h8"/><path d="M9.5 20.5h5"/></g></svg>`)
	iconLampOff = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M7.2 3.5h9.6l-1.7 6.4H8.9L7.2 3.5Z"/><path d="M12 9.9v7.6"/><path d="M8 17.5h8"/><path d="M9.5 20.5h5"/><path d="M4 4l16 16"/></g></svg>`)

	iconLatencyLow      = iconSVG(`<svg viewBox="0 0 24 24"><path d="M13.2 2.2 5.4 13.1h4.7l-1.2 8.7 8.1-11.2h-4.8l1-8.4z" fill="currentColor"/></svg>`)
	iconLatencyStandard = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="8" stroke="currentColor" stroke-width="1.7"/><circle cx="12" cy="12" r="4" stroke="currentColor" stroke-width="1.7"/><circle cx="12" cy="12" r="1.6" fill="currentColor"/></svg>`)
	iconLatencyStable   = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="5" r="2.5"/><path d="M12 7.5V20M8.5 10.5h7"/><path d="M4.5 14a7.5 7.5 0 0 0 15 0"/><path d="M4.5 14l-2 2M4.5 14l2 2M19.5 14l-2 2M19.5 14l2 2"/></g></svg>`)
	iconStopwatch       = iconSVG(`<svg viewBox="0 0 24 24" fill="none"><g stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="13" r="7"/><path d="M12 10v3.2l2.1 1.7"/><path d="M9.5 2.8h5M12 2.8v3.2"/></g></svg>`)
	iconScreen          = iconSVG(`<svg viewBox="0 0 20 20" fill="none"><g fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="2.5" y="4" width="15" height="10" rx="1.6"/><path d="M7 17h6"/></g></svg>`)
)

func iconSVG(source string) *ui.SVG {
	return ui.MustParseSVG([]byte(source))
}

// faceIcon returns the state face named by the tray's face string. The
// resting smile is the app's plain mark.
func faceIcon(face string) *ui.SVG {
	switch face {
	case "stream":
		return faceStream
	case "connect":
		return faceConnect
	case "error":
		return faceError
	case "idle-open":
		return faceIdleOpen
	default:
		return iconMark
	}
}
