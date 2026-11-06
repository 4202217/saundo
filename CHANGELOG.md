# Changelog

All notable changes to saundo are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and this project uses
semantic versioning. `mygo build` reads the `## <version>` section as the
release notes.

## 0.5.0 - 2026-11-07

### Added

- The Now Playing card in the flyout. It reads the Windows media session
  and shows the app, cover, title, artist, a live five-band meter, elapsed
  and remaining time, and the transport controls the session reports. The
  session-switch arrow moves between open media sessions.
- The Now Playing settings page. It controls the card's display master,
  the cover mode (none, default, or the media artwork), cover colors, the
  audio level, playback controls, and the receiver track-info push.
- Ambient light. Six palettes plus a default reset paint the top display
  of a HomePod on software 27.2 through the Now Playing cover. Choose the
  preset in the Ambient light settings page or the flyout's ambient card.
  With the light off the receiver keeps its default or the media cover.
- Receiver cover art. The media thumbnail now reaches the receiver as an
  image on the encrypted control channel. The ambient image and the
  track-info cover share one slot; the light takes it while it is on.
- Mute the PC output. A connection switch mutes the local render endpoint
  while a session streams and restores its earlier state on disconnect,
  quit, or when the switch is turned off. A guard file undoes the mute
  after a crash. The settings note warns when the driver lets WASAPI
  loopback capture survive the mute.
- Appearance. The General page follows Windows, light, or dark, and both
  windows repaint when the choice changes.
- The About page. It shows the version, the project links, the GPL
  license, and the third-party notices.
- Directed discovery. A unicast pass over the ARP neighbors and the
  remembered hosts finds receivers that miss the multicast browse, so a
  HomePod that never answers multicast is still found and can be reached
  after a restart.

### Changed

- The flyout and the Receivers page list only receivers that can be used:
  manual entries, online discovered receivers, and the active session's
  row. An offline discovered receiver's saved settings return with the
  device.
- The Receivers page renders an empty list as plain prose, with the add
  row as the only plus, and collapses hidden rows when their group closes.
- The meter gates band energy below -50 dBFS and floors each band's gain,
  so dither and room tone no longer fill the bars during quiet passages.
- The update notes dialog reads the offered version's entry from the
  localized changelog, so a Chinese dialog no longer shows English notes.
- A local build reports its git describe stamp, and the updater still
  offers the release that the build sits past.
- The app now builds on mygo 0.8.0.

### Fixed

- The cover art now reaches the receiver. The sender emitted the image,
  but the engine passed an empty thumbnail and the app's change key held
  only the text.
- A song change no longer replaces the ambient light, and turning the
  light off hands the cover slot back to the media artwork or the origin
  image.
- A stereo group name arrives as RFC 1035 byte escapes in the TXT `gpn`
  field and is now decoded, so a UTF-8 name such as the Chinese for
  "bedroom" no longer appears as escapes.
- A stale media thumbnail, such as Chrome's placeholder logo, is replaced
  when the artwork changes under the same title.
- The disconnected idle tray face is an open-eyed line face, distinct from
  the silent-session smile.
- Connecting to a remembered receiver without a host or port fails closed
  instead of dialing an empty address.

## 0.4.2 - 2026-11-04

### Added

- Receiver management on the settings Receivers page. Add a receiver by IP
  or hostname when mDNS cannot find it, edit or remove rows, hide them from
  the flyout, and set one auto-connect target. Each receiver keeps its own
  DSP/DAC delay and a password stored with DPAPI. The new `receivers.json`
  registry persists discovered receivers across a power-off.
- Latency modes. Low 200 ms, standard 1.5 s, stable 2 s, or a custom value
  from 20 to 2000 ms, chosen in the flyout or on the Audio page. The ALAC
  line shows the resolved latency with the receiver's own delay, and a live
  change restarts the stream with a spinner while the last streaming state
  stays on screen.
- Tray animation. The connect and stream faces animate at 12 frames per
  100 ms. The General page has a Tray animation switch, on by default, and
  Windows animation effects off holds the static face.
- Updates in settings. The Updates page shows available, downloading, ready,
  and failed states plus the release-notes history. The notes dialog follows
  the app language, and a found update appears as a flyout chip and a tray
  tooltip without taking focus.
- Stereo groups prompt for a member's receiver password and reconnect the
  pair after it is entered.

### Changed

- Settings are written once after a short delay instead of on every volume
  tick or toggle, so a slow disk no longer stalls playback.
- Diagnostics rotate by byte count, and the sender reports pacer lag,
  dropped host events, and bounded teardown write failures.
- Quitting no longer waits for an in-flight mDNS query, so the window closes
  without a freeze of up to 1.5 seconds.
- UDP sockets use a 256 KB receive buffer for timing and retransmit bursts.
- Restarting capture stops the previous source on its own goroutine, so it
  no longer blocks the playback loop.

### Fixed

- A stale PIN dialog is cleared when another device connects.
- A reconnect retry no longer tears down an active session.
- A stereo group whose member drops before launch now fails cleanly and
  reconnects instead of staying half-dead.
- Changing latency while a PIN dialog is open no longer cancels the pairing,
  and an offline receiver is reported instead of silently keeping the old
  session.
- The tray and flyout show the resting face when a connected stream has been
  silent for two seconds.
- A receiver that rejects stored pairing credentials now pairs again on the
  same connection instead of failing every retry.
- The receiver page keeps its selected row, expanded state, and scroll
  position across refreshes, and the hidden-group animation no longer
  squeezes the enabled card.

## 0.4.1 - 2026-11-02

### Fixed

- The volume readout now shows 0% to 100%, not 0% to 10000%.
- Dropped samples are shown only while streaming; idle capture no longer
  reports a growing count.
- Non-ASCII sender names are encoded as UTF-16BE in the AP2 session plist,
  so a Mac receiver shows the Chinese name instead of mojibake.
- The settings window uses one background across its title bar, sidebar, and
  content.

## 0.4.0 - 2026-11-02

### Added

- English and Simplified Chinese interfaces, with a language setting and a
  system-language default. The tray menu, flyout, settings, dialogs, and
  screen-reader labels follow the selected language.
- Typed message catalogs in `cmd/saundo/locales`, generated by mygo, with a
  key-parity test and `mygo i18n check` in the gate.
- English and pseudo-locale (`en-XA`) rendering tests.

### Changed

- Interface copy now comes from the catalogs; code and diagnostics remain
  English.
- The project documentation is split into architecture, protocol, devices,
  UI, implementation, and verification.
- `make check` now validates the catalogs and the mygo UI rules.

### Fixed

- Window lifecycle, keyboard operation, and idle-frame work from the
  accessibility branch.
