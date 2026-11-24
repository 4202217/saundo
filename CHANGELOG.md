# Changelog

All notable changes to saundo are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and this project uses
semantic versioning. `mygo build` reads the `## <version>` section as the
release notes.

## Unreleased

### Added

- The connection page is a live send-side dashboard. The measured card reads
  the latency against the receiver family's measured floor, the ring headroom
  with the trailing minute's low, the window's underruns split between a
  combo's sides, and the session's L4 traffic with its packet size. A latency
  below the floor adds a warning banner and points at the audio page; the
  channel and volume left the page for the tray panel's mixer. A 60-second
  strip plots the headroom and the two event kinds, marks a silent stretch
  with a labeled band, labels the current value at its right edge, and names
  the latest event with its age and count; the device rows carry each
  member's headroom, rate, and the age of its last underrun.

## 0.6.6 - 2026-11-23

### Fixed

- Pauses no longer crackle or force a reconnect. The stream never stops while
  the session is up: a fully starved packet carries a 16-byte compact ALAC
  silence frame (352 zero frames through the rice coder's zero-run case), so
  a silent stream costs about 80 kbps per receiver instead of 1.4 Mbps and
  the receiver's jitter buffer and renderer never starve. The idle state and
  the standby clock still start five seconds after the last sound; the pacer
  never stops while the session is up.
- The underrun counter reports a short fill only when the source comes back
  within the window. A paused source's silence run is no longer counted, so
  pausing a track no longer reads as stuttering.
- A full-scale transition between adjacent samples no longer wraps into a
  click at maximum volume, and a ring reset no longer replays one packet of
  pre-reset audio.
- A receiver password is never offered to a receiver that has not proven the
  pairing: a failed pair-verify signature now ends the session when a
  password is in play, because that response is offline-crackable.
- A stalled event channel no longer drops control data and kills the session
  as an authentication failure: TCP backpressure absorbs the stall, a
  connection whose writer died reports later sends as failed, and replies
  match their request by CSeq instead of shifting every later handler.
- The keep-alive is armed only by a feedback reply, so a rejected command no
  longer disables it. Eight consecutive event-channel decrypt failures end
  the session instead of streaming mute, and a lost volume reply recovers
  after three seconds.
- A clean control close counts as a takeover only when the event channel
  worked this session, so an AirPlay 1 session retries instead of reading as
  taken over.
- PTP timing follows the transport, not the pairing method: an AirPlay 2
  HomePod or Mac that pairs by password, or advertises no HAP bits, now gets
  PTP timing instead of an NTP stream the receiver silently ignores.
- A clean disconnect rebases the drop accounting, so the next connection no
  longer counts pre-stream overflow as stutter, and switching to per-app
  capture releases an in-flight local-output mute instead of leaving the
  computer muted until the next launch.
- A locked Keychain no longer quarantines the credentials file: loads serve
  the last known store, writes refuse, and a transient read failure no longer
  reads as "no password" and re-pairs the receiver.
- A member lost from a multi-device session leaves the wanted set even when
  the survivors cannot rebuild, so a reconnect no longer retries a receiver
  that is gone or has been taken over.
- The password prompt submits the secret exactly as typed, including spaces.
- In English, the connection card's stability label keeps its width and the
  60-second note carries the ellipsis.
- The hidden-receivers row's click target matches its pill, and the flyout
  device head sits as close to its card as the ambient head.

## 0.6.5 - 2026-11-22

### Added

- Multi-device playback. One capture streams to several receivers at once as
  a member set: each target gets its own flyout row with its own volume and
  a stereo, left, or right channel; adding, cancelling, or disconnecting one
  target rebuilds the rest without ending the session. A member that dies
  leaves the session and the survivors keep playing, with a notice bar
  naming the loss and offering a retry. The connection page states the
  session: the connected devices with their channel and state, and the
  measured health of this connection.
- Software stereo combos. On the receivers page, link any two AirPlay 2
  receivers into one stereo target, across the models and generations
  Apple's official pairing won't mix: playback order with a hot side swap,
  one volume for both, in-place editing and unlink, and a launch
  auto-connect that starts the combo. An official stereo pair made in the
  Apple Home app wins over a combo of the same speakers: the combo reads
  Unavailable until the pair is dissolved, and its members return as
  standalone rows.
- A Model detail switch on the receivers page. Off by default, it hides the
  parenthetical in model names: MacBook Air (M2, 2022) becomes MacBook Air.

### Changed

- Launch auto-connect accepts several receivers. Toggling one receiver no
  longer clears the others; the launch connects every flagged receiver as
  one cohort. A receiver linked into a combo hands its flag to the combo.
- The stability verdict and the retransmit and underrun counters read the
  trailing 60 seconds, so a join's padding blip clears instead of pinning
  the card for the whole session.
- Receiver rows show the state their title needs: a stereo pair's online
  count, the password and authentication chips, and the launch chip; the
  second line carries the address, channel, model, and system version. The
  flyout is the only surface that keeps model identifiers.
- The receivers page gives each section its create action: standalone
  receivers add from the section head, stereo combos create there, and an
  empty section keeps one dashed row. The enabled section is renamed
  Standalone receivers.
- The flyout playback head shows only the effective latency; the codec and
  sync notes leave the line. Flyout rows drop their selection tint: rows
  are peers, and the device section head carries Disconnect all for two or
  more live targets.
- The flyout's latency segments center their icon-and-label pairs,
  matching the lamp, channel, and cover controls.

### Fixed

- Launch auto-connect no longer waits forever for a flagged receiver that
  never reports an address: after a grace period it drops that row and
  connects the rest, and the dropped row keeps its flag for the next start.
- A receiver that answers from more than one network address keeps its event
  replies inside a multi-device session: the cohort routes them by the
  connection they arrived on, not by the address it dialed.
- Connecting with nothing playing no longer reports phantom underruns:
  silence padding counts only while the ring is being fed.
- A receiver that refuses its saved password opens the password prompt
  instead of tearing the session down; the survivors keep playing and no
  retry ladder runs.
- Dissolving an official stereo pair now de-synthesizes the pair within one
  discovery cycle, and a member's own row returns when the pair stops
  advertising.
- Apple Music's idle thumbnail, a transparent frame with no opaque pixel,
  no longer draws a white line around the cover placeholder.
- Combo corrections: the create picker excludes the receiver the other side
  already holds, a stereo target drops its channel button, swapping sides
  twice restores the original sides, a pair member cannot be linked or
  launched alone, and adding a receiver that just went offline is refused
  before a healthy cohort stops.
- The audit's surface fixes: the combo create entry waits for two eligible
  receivers, a refused save, link, or add reports on the receivers page, a
  dismissed or expired notice resizes the flyout, a collapsed offline combo
  reads Offline, and the tray draws the mixed-size combo glyph.
- Chips and mixer marks sit level with the text beside them.

## 0.6.4 - 2026-11-17

### Added

- Media controls in the tray menu. While the system has a media session, the
  right-click menu offers previous, pause or play, and next after the
  receiver rows.

### Changed

- The tray menu is rebuilt around the session. Up to six receiver rows
  connect or switch, the in-session row is disabled, and one session row
  carries cancel, disconnect, or retry. More than six receivers open the
  receivers page, and an empty menu offers a rescan. The ambient light
  becomes a submenu with a preview of every choice, a check on the current
  one, and Last used on the stored choice while the light is off. The update
  row appears only when an update can be downloaded or installed. Local mute
  leaves the menu; the Connection page keeps the switch.
- The interface follows the desktop text scale. At 125% and above both
  windows zoom as a whole, so labels, values, and rows grow together instead
  of clipping; high contrast keeps the design palette.
- The media card's head places its app icon, name, and session switch on one
  top-aligned line, with an 18-DIP icon and a 20-DIP switch on the card's
  10-DIP inset.

### Fixed

- A cold start to a stereo pair lights both top displays. A remembered pair
  keeps its members' OS version before discovery answers, and a pair member
  announces playback before its first cover, so the second display no longer
  stays dark until its glass is tapped.
- Ambient light changes made during playback reach the second HomePod's top
  display again; the pair's member sync is seeded at session start.
- Retry after the automatic reconnect attempts are used up starts a fresh
  backoff. A failed manual retry no longer leaves the row permanently
  stopped.

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
- The English flyout's latency segment shows the single word Low instead of
  a truncated label, and the capture page says System audio, or one or more
  apps. instead of a single app.

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
