package app

import "saundo/internal/lamp"

// window_scale.go.

const (
	// flyoutHeight is the height with three device rows, no playback section,
	// and no error bar. The device head reserves its 26-DIP action slot in
	// every state, so the base includes the height that slot adds. The media
	// card and the latency row add their own heights; the playback section's
	// head is only drawn when one of them shows.
	flyoutWidth  = 420
	flyoutHeight = 438

	// errorBarHeight is a status bar under the device card and the body gap
	// above it: the connection error and the credential notice share it.
	errorBarHeight = 52

	// The media card fills the space the old 62-DIP Now Playing row used, and
	// adds its own height when shown.
	nowPlayingRowHeight  = 62
	nowPlayingCardHeight = 150

	// The device card shows one row per device, up to deviceRows, and the
	// empty state's box when there is none. Every row keeps one height,
	// connected or not.
	deviceRows        = 3
	deviceRowHeight   = 66
	deviceEmptyHeight = 141

	// latencySwitchHeight is the four-segment row its divider leaves in the
	// playback card.
	latencySwitchHeight = 44

	// playbackSectionHeight is the section head, its card borders, and one
	// body gap: the height the playback section would hold with nothing to
	// show, so the flyout drops it when neither media nor the latency row is
	// present.
	playbackSectionHeight = 33

	// sessionNoticeHeight is the notice bar under the device card and the
	// body gap above it.
	sessionNoticeHeight = 52
)

func deviceCardHeight(n int) float32 {
	if n <= 0 {
		return deviceEmptyHeight
	}
	return float32(min(n, deviceRows))*deviceRowHeight + 2
}

// flyoutHeightFor is the window height for the visible device count, the
// current media state, and the permanent ambient section. The device card and
// the playback card touch through the body's gap; there is no flexible
// spacer. A shown media card adds its height, and the no-media panel is
// shorter by the same amount. The ambient section is always present.
func flyoutHeightFor(n int, showLatencySwitch, hasMedia bool) int {
	h := flyoutHeight - int(deviceCardHeight(deviceRows)-deviceCardHeight(n)) - nowPlayingRowHeight
	if hasMedia {
		h += nowPlayingCardHeight
	}
	if showLatencySwitch {
		h += latencySwitchHeight
	}
	if !showLatencySwitch && !hasMedia {
		h -= playbackSectionHeight
	}
	h += lampSectionHeight
	return h
}

// flyoutHeightForState adds the error bar and the template grid while they
// are shown. The window is anchored by its bottom edge, so this grows it
// upward. The device head's action slot is already in the base height.
func flyoutHeightForState(n int, showLatencySwitch, hasMedia, lampCover, lampExpanded, errorBar, credentialBar, notice bool) int {
	h := flyoutHeightFor(n, showLatencySwitch, hasMedia)
	if notice {
		h += sessionNoticeHeight
	}
	if errorBar {
		h += errorBarHeight
	}
	if credentialBar {
		h += errorBarHeight
	}
	if !lampCover && lampExpanded {
		h += lampGridHeight(len(lamp.All()))
	}
	return h
}
