# Changelog

All notable changes to saundo are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and this project uses
semantic versioning. `mygo build` reads the `## <version>` section as the
release notes.

## 0.6.3 - 2026-11-14

### Added

- Per-app capture on the Mac. The Capture settings list running applications
  with their icons; pick one or more, and the selection streams through one
  tap. Audio helpers fold into the app whose bundle ID prefixes theirs. The
  selection keeps bundle IDs, so it survives restarts. The capture status
  names the selection count and warns when an app is missing or is waiting
  for its first sound.
- Quick actions in the tray menu. A right click lists one row per visible
  receiver, with the connected receiver in the accent color; when no row is
  the target, Connect to last receiver follows. Mute local output and the
  ambient light toggle from the same menu, and the mute row reads Unmute
  local output while the preference is on.

### Changed

- Connecting another receiver while streaming switches to it in place. The
  local mute survives the switch, and the previous output comes back if the
  new session fails. While streaming, the panel offers Connect on the other
  online rows.
- The local mute survives a latency change that restarts the stream.
- The playback card orders media, latency, and volume on one content column.
  The volume row shows its integer percent on one line, the latency glyphs
  are larger, and the transport glyphs draw their circle only on hover.
- The latency preset cards share one height, so a wrapped description no
  longer leaves them uneven. The connecting row's action carries a cancel
  icon.
- The tray menu no longer offers Disconnect; the panel's active receiver row
  keeps it.

### Fixed

- An unknown receiver volume no longer means full scale. When the AP2
  handshake has no usable initial volume, or reports -144 (mute), the sender
  sends no volume command and the receiver keeps its own level. A malformed
  value is no longer read as zero, a value below -144 clamps to mute, and the
  1 Hz volume poll follows the same rule.

## 0.6.2 - 2026-11-12

### Added

- The Mac build updates in place from the signed release feed, the same path
  Windows uses. Until a Developer ID signs the build, an update can ask again
  for system-audio capture and Keychain access.
- A saved-pairing notice. When the app cannot read the pairing data, the tray
  panel shows a notice under the receiver card; its dialog explains the reset
  and offers to pair again or stop showing it. The unreadable file is moved
  aside and the credential store is rebuilt, so the next pairing can save its
  key.

### Changed

- Every engine error follows the app language. An English install no longer
  shows Chinese text for a failed connect, receiver command, or update.
- A new install advertises its sender name in the app language.
- Every capture endpoint stays selectable. Duplicate names get a number
  suffix, and a follow-system snapshot no longer leaves a stale device label.
- The tray menu icons are drawn from the interface's vector set.
- The Mac build holds off App Nap while connecting, pairing, streaming, or
  reconnecting.
- The pairing dialogs fit inside the 420-wide tray panel, fractional sample
  rates print as 44.1 kHz, and the add-receiver hint no longer ends with a
  period.

### Fixed

- The Mac build mutes the local output while connected. The mute had failed
  on every connection, so the local speakers kept playing alongside the
  receiver.
- The Mac capture path does not lock or allocate on the audio thread, and the
  level analysis runs off it, which keeps capture smooth under load.
- A retransmit request is answered only to the session's receiver and is
  bounded, so a forged request cannot reflect audio to another address.
- Session keys are cleared when a session ends, and an audio packet that
  cannot be encrypted is dropped instead of sent in clear.
- A bad event frame no longer discards the frames that follow it.
- The frame clock keeps running past 4.84 days.
- A failed initial capture open retries, a failed settings write stays
  pending, and a full queue no longer drops a lifecycle event.
- Corrupt or newer receiver and settings files are never overwritten.
- Discovery keeps the local endpoint, rejects IPv6-only advertisements, and
  runs its directed pass off the publish path.
- PTP rejects an overflowing timestamp and an IPv6 peer, and the follower
  delivers packets outside its lock.
- The no-tray fallback window resizes with its content, and the receiver
  delay timer stops on quit.
- The Now Playing adapter's child process is bounded and its self-test
  retries; Now Playing bookkeeping stays on the main thread.

## 0.6.1 - 2026-11-11

### Added

- Ambient light idle standby. After the chosen period without playback
  (Never, 1, 5, 10, or 30 minutes; 30 by default) the HomePod top display
  turns off, and the first sound turns it on again. The tray panel shows a
  Standby chip, and its footer says the display wakes when sound plays.
- The connection page health card. It shows the receiver's own reported
  processing time and the session's stability. A stereo pair reports the
  higher member's time; a receiver that reports none reads Not reported.
  Latency mode and receiver processing measure different stages and are
  never added.
- The filled receiver glyphs for HomePod, HomePod mini, Apple TV, AirPort
  Express, and the Mac laptop, mini, studio, and desktop forms. A Mac's form
  follows its model identifier.
- Session takeover. When another device takes over the receiver, the app
  shows Taken over by another device and offers Reconnect instead of
  reconnecting in a loop.
- A receiver that runs on this computer is never a target. It appears in the
  hidden group with a Local chip, is never auto-connected, and the app
  refuses to connect to it.
- The diagnostic log moves to the About page, which opens it over the
  settings window.

### Changed

- Launch is faster. The receiver group, its features, and its authentication
  are cached, so auto-connect starts without waiting for discovery.
- The sender chooses one, two, or three packet copies from the last
  session's retransmit rate and sends two prefill packets, which cuts
  retransmits on a lossy link.
- The macOS tray panel style setting is removed; the rounded flyout is the
  only style.
- The receiver password row appears only for a Mac, a manual receiver, or a
  row with a saved password that can still be cleared; a PIN-paired HomePod
  or Apple TV with no saved password hides it.
- The tray panel footer carries the face, the phase, and the update chip.
- Settings row hints carry no final period, and the language and appearance
  subtitles are shorter.

### Fixed

- The update channel stays on the chosen setting after a restart instead of
  reverting to Beta.
- A HomePod that uses an AirPlay password keeps the HomePod profile; the
  receiver profile follows the receiver's family, not its pairing method.
- A capture device that becomes invalid retries with growing delays and logs
  at most once a minute instead of every two seconds.
- The stability readout counts only drops from the current session, so an
  idle capture overflow no longer marks a clean session as Stuttering.
- On a network without IPv6, discovery no longer floods the log with bind
  failures.

## 0.6.0 - 2026-11-09

### Added

- macOS support on Apple Silicon (macOS 14.4+). System audio is captured
  with a Core Audio process tap, credentials use the login Keychain, the
  media card reads the system media session through a bundled MediaRemote
  adapter, and the build ships as an app bundle and a dmg. The mac build is
  ad-hoc signed and its updater stays off until a signed, notarized feed
  exists.
- The media card names the app playing and shows its icon, resolved from
  the Windows shell's Applications folder, then the running executable's
  version resource. An app without an icon shows the name alone.
- The media card follows the system's current media session. The switch
  arrow pins another open session until a different one becomes current.
- The flyout style setting (macOS only): a system popover or the rounded
  flyout.

### Changed

- The app is named Saundo in the interface, the window titles, the
  installer, and the About page.
- The Now Playing settings page is the Media card page. Its switch reads
  Show in tray panel and only hides the card in the tray panel; the
  preview and the option rows stay active. The cover options read None,
  Default, and Media.
- The Ambient light page has a Color source row (Cover or Template), and
  its six template swatches keep one size and wrap on a narrow page.
- The settings content column caps at 720 DIP and centers on a wide
  window. The Audio page stacks the three latency presets as full-width
  rows below 714 DIP, so no name, value, or description is cut. Caveats
  sit under their cards as info notes.
- The tray menu item reads Show tray panel, and receiver copy replaces
  device copy throughout.
- The About page names the license GPL-3.0-only.
- The Windows update feed stays on; the mac build disables its updater.

### Fixed

- After a failed session, the other online receivers stay connectable
  instead of showing a disabled Connect.
- Cancelling a connection clears the previous failure instead of leaving
  a stale banner.
- A receiver discovered through the other AirPlay service no longer keeps
  the service suffix in its name.
- The lamp preview follows the session the app pushes to the receiver,
  not the card's arrow selection, so the preview and the top display
  cannot disagree.
- The window title follows a language change.
- On macOS, the capture tap no longer reopens on a false device-stopped
  signal; the aggregate liveness probe was removed.
- The tray panel style setting survives an engine snapshot, so the toggle
  no longer appears to revert.
- The tray panel's update-ready chip uses the ready wording instead of the
  restart action.
- The release copy review's fixes: the capture description and receiver
  port hint fit at the default width, the macOS build says computer and
  local output instead of PC, the update-failed line keeps its reason
  localized, and the receiver launch chip, Chinese receiver and license
  terms, and About notices match the copy standard.

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
- The local-output mute, ambient light, and receiver track-info switches
  now default on.

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
