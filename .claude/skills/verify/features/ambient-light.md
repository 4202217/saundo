# Ambient light

The HomePod top display renders the current Now Playing cover. The ambient
light is a permanent global card: a desk-lamp glyph and a power action turn
the top display off and on, and a two-way priority segment chooses between
the media cover and one of six generated templates. One global choice, not
per receiver.

## Sub-features

- 氛围灯 settings page and the flyout's ambient card. The card is always
  present; the flyout head carries the section label and the power action
  (关灯 when lit, 开灯 when dark), while the desk-lamp glyph is on the
  settings card head and the settings sidebar, not in the flyout head. The
  row carries the compact 封面/模板 priority
  segment left of the chevron. The settings page puts a 颜色来源 row
  (封面 / 模板) at its right, under the 72-DIP preview and its name, and its
  head keeps the HomePod 专属 tag. The tray menu has no
  ambient item.
- Choices: the media cover (default) plus six templates: 暖烬, 沙丘, 苔庭,
  潮汐, 兰夜, 月白. `internal/lamp` renders each palette as a deterministic
  600×600 carrier of three flat color fields and caches it; the receiver's
  Lab k-means reduces the carrier back to the three colors. The cover has no
  rendered image.
- The preview follows what the top display follows: the session the app
  pushes (the system's current session), not the media card's arrow
  selection — switching the card to a paused session leaves the preview on
  the pushed cover. Cover priority runs the cover through the same Lab
  k-means the receiver uses for `MediaColors` (`lamp.CoverPreset`) and
  paints the three-color glow. With no cover yet it
  paints a neutral cover tile with the media glyph, and the row shows the
  无媒体 / No media chip (`lamp.go:307-321`); the receiver's own
  fresh-session placeholder is a white center on a dark ring and is not
  previewed. Template priority paints the selected template's three-color
  glow; the carrier itself is not shown.
- In cover priority the flyout row shows the preview, the current source and
  the segment: no chevron and no template list. Choosing 模板 shows the
  chevron; the six swatches stay collapsed until it is clicked. Picking a
  template keeps template priority and leaves the flyout list open. Hiding
  the flyout folds the list, so the next open starts collapsed. The settings
  page keeps the six templates visible in both priorities.
- While the light is off the preview dims to 35% like the templates, the name
  and a 已关灯 chip mark the state, the priority segment and the templates
  are disabled, and only 开灯 works. Turning the light on restores the
  current choice and leaves the template list collapsed.
- A template leaves as one `SET_PARAMETER image/jpeg` cover on the encrypted
  control channel. It is sent when the choice changes and when a stream
  starts; the artwork POST command is not used.
- The light follows the cover, not the play/pause state: a paused track
  keeps the current light, and a new cover (a track change) updates it. The
  manual 关灯 action and the idle standby are the two ways to darken the
  display; a media pause itself does not.
- 关灯 posts the updateMRPlaybackState command with `mrPlaybackState` 2;
  开灯 posts state 1 and then resends the current cover. A new session
  assumes playing and does not send a redundant state 1. The state is
  persisted as `lamp.off`; the light is on by default, and an old
  `lamp.enabled: false` migrates to off.
- Cover priority is persisted as `lamp.cover`. An old `preset: "media"` or
  an unknown/empty preset turns it on and resets the template to `dune`; a
  valid old template keeps it off. The priority segment and the template
  grid write the same flag.
- While the light is off, picking a template or the cover segment does
  nothing and the grid is disabled; only 开灯 restores the current choice.
  `TestLampOffIgnoresPriorityAndPreset` pins the early return.
- With cover priority on and no media cover, the sender stops sending an
  image and the receiver keeps its last light. A fresh session with no
  artwork shows the receiver's white-center placeholder. The sender has no
  clear-artwork command, so the placeholder is not reachable after a
  template or cover has been shown without a new session.
- One artwork slot: cover priority hands it to the media cover and a
  template owns it; the playback state is independent. A media cover change
  updates the text without displacing a template.
- Capability gate. `device.TopDisplayCapable` accepts any `AudioAccessory*`
  model at software 27.2 or later (6,1 included; there is no model
  whitelist) and fails closed on an unknown model or version. A stereo
  group fans the image out to each capable member; the playback state goes
  to every member.
- Sound Glow must be enabled on the HomePod's own settings. Without it the
  top stays white.
- Idle standby: one row at the bottom of the settings page, 自动关闭氛围灯,
  with a 从不/1/5/10/30 minute select (default 30). Five seconds of digital
  silence stops the RTP pacer, and once the configured idle time has passed
  since the last push the engine sends playback state 2. The first audible
  sample sends state 1 and resumes; pressing 开灯 while the display is dark
  relights it immediately and holds it on until the next sound
  (`lamp.go:226-236`; `engine.go:1152-1155`). The flyout chip reads 待机,
  the power action 开灯, and the footer 已待机 · 播放声音即唤醒.

## How to get to it (user POV)

设置 → 氛围灯, or the ambient card under the playback card in the flyout.
The power glyph turns the top display off and on; the 封面/模板 segment in
the flyout row, left of the chevron, and the 颜色来源 segment (封面 / 模板)
on the settings page choose the source. With 封面 chosen the flyout has no
template list; choosing 模板 reveals the chevron; the six swatches stay
collapsed until it is clicked.

## Driving it

Headless UI:

```bash
go test -run 'TestFlyoutLampCardStaysWhenOff|TestFlyoutLampExpandsAndChoosesPreset|TestFlyoutLampOffRequiresTurningOn|TestLampOffIgnoresPriorityAndPreset|TestFlyoutLampCoverHidesTheGrid|TestFlyoutCloseCollapsesLampList|TestSettingsLampPrioritySegmentTurnsCoverOn|TestSettingsLampPrioritySegmentTurnsCoverOff|TestSettingsLampPickingPresetWhileCoverOnTurnsCoverOff|TestSettingsLampPowerTurnsTheLightOff|TestSettingsLampPageChoosesPreset|TestSettingsLampPageOffRequiresTurningOn|TestLampSurfacesInEnglish|TestLampTemplateTilesKeepTheirSize|TestLampPreviewFollowsThePushedSessionNotTheCard|TestSettingsLampStandbySelect|TestFlyoutLampShowsStandby' -v ./internal/app/
```

Renderer, cover palette, and settings:

```bash
go test -run 'TestPresetsMatchCatalog|TestGeneratedJPEGsAre600Square|TestPresetCarriersRecoverTheirPalette|TestPresetColorsAreDistinct|TestPresetWeightsSumToOne|TestGeneratorCachesPerPreset|TestCoverPresetClustersTheCover|TestCoverPresetRejectsEmptyImages|TestCoverPresetIsDeterministic' -v ./internal/lamp/
go test -run 'TestTopDisplayCapable' -v ./internal/airplay/device/
go test -run 'TestLampDefaultsOnAndCover|TestLampOldFileLoadsDefaults|TestLampLegacyEnabledInvertsToOff|TestLampOldMediaPresetTurnsCoverOn|TestLampOldTemplatePresetTurnsCoverOff|TestLampRoundTrips' -v ./internal/settings/
```

Sender and engine:

```bash
go test -run 'TestSetLampArtworkSendsCover|TestSetLampArtworkWaitsForStreaming|TestSetLampArtworkDuplicateIsNoOp|TestSetLampArtworkDoesNotTouchDMAPText|TestSetLampArtworkClearSendsNothing|TestSetPlaybackStateSendsCommand|TestSetPlaybackStateWaitsForStreaming|TestSetPlaybackStateAssumesPlayingOnSessionStart|TestSetPlaybackStateResumesWithTheCover|TestMediaCoverWinsWhenLampIsOff|TestLampCoverWinsWhileOn' -v ./internal/airplay/sender/
go test -run 'TestLampOffOnAppliesAndPersists|TestLampAppliedWhenStreamingStarts|TestLampUnknownPresetFallsBackToDefault|TestLampCoverKeepsPlayingOnPause|TestLampSkipsIncapableReceivers|TestLampTogglingTwiceLogsTheSkipOnce|TestGroupArtworkFansOutToBothCapableMembers|TestGroupArtworkSkipsIncapableMember|TestStandbyDarkensAtThresholdAndResumesOnSound|TestLastPushTracksTheStreamNotSilence' -v ./internal/engine/
```

Live, with a HomePod at software 27.2 or later and Sound Glow on:

1. Connect and stream, then open 设置 → 氛围灯. Cover priority is on by
   default; the preview follows the current cover and the top repaints
   within a second or two. The flyout card shows the preview, 封面 and the
   segment, with no chevron and no template list.
2. Choose 模板 in the flyout row. The chevron appears; click it to show the
   six swatches, then pick 潮汐 and the top repaints. The list stays open.
   Close and reopen the flyout; the list is collapsed again. The settings
   page shows the same change.
3. Choose 封面. The chevron and the swatches hide; the top follows the
   cover again.
4. Pause the track. The light stays on the current cover.
5. Press 关灯. The top goes dark; the card stays and the glyph shows 开灯.
   The preview dims like the templates, the segment and templates disable,
   and the chip reads 已关灯. A template cannot be picked while off.
6. Press 开灯. The top shows the current cover or template again; only now can
   the segment and templates be changed.
7. With the light on, pick another template in the settings page. The
   priority becomes template.
8. Restart the app while the light is off. It stays off; 开灯 restores it.
9. Repeat with a stereo pair: each capable member repaints and goes dark.

Both probes need the app stopped: they bind PTP UDP 319/320, which a running
instance owns. Do not run them while it streams.

Idle standby on a stereo pair (verified 2026-11-10):

1. Set 设置 → 氛围灯 → 自动关闭氛围灯 to 1 分钟.
2. Connect, play, and confirm both displays are lit.
3. Stop playback and stay quiet. After 5 s the log reads
   `audio idle for 5s, stream suspended`; one minute after the last push it
   reads `standby: lamp off after 1m0s idle on HomePods`, and both displays
   go dark. The session stays up; the flyout chip reads 待机 and the footer
   `已待机 · 播放声音即唤醒`.
4. Play a sound. The log reads `audio resumed` and `standby: lamp on`; both
   displays light and the audio resumes.
5. Set the select back to 30 分钟. The flyout is hidden while the settings
   window is open; capture it separately if the chip is the evidence.

The playback-state probe (muted, 27.2) is:

```bash
SAUNDO_PLAYBACK_STATE_TEST=1 go test ./internal/airplay/sender -run TestLivePlaybackState -v
```

The no-media placeholder probe (muted, stereo pair 27.3) is:

```bash
SAUNDO_PLACEHOLDER_TEST=1 go test ./internal/airplay/sender -run TestLivePlaceholderReturn -v
```

It sends a tide template, clears the artwork slot, cycles playback state
2→1, and sends a new item without artwork. The observed result is blue and
dark, with no white center: the receiver keeps the last image. The white
center appears only on a fresh session with no artwork.

Proven end state: the priority segment and the six templates share one
global choice; cover priority hides the flyout chevron and template list;
template priority shows the chevron and expands the list only on click; the
preview follows the cover's extracted
palette; each template carrier yields its three designed colors under the
receiver's k-means, and the template preview paints those three colors, not
the carrier; the app preview shows a media-glyph tile when there is no cover;
the receiver's fresh session shows its white-center placeholder; after a
template, the last light survives clearing the slot; the power action
darkens and restores the display without stopping the stream; the choice
and the off state survive a settings-window close/reopen and an app restart.

## Gotchas

- The `SAUNDO_PLAYBACK_STATE_TEST` and `SAUNDO_PLACEHOLDER_TEST` probes need
  the app stopped: they bind PTP UDP 319/320, which a running instance owns.

- Sound Glow off leaves the top white. That is the HomePod setting, not the
  app; the app cannot enable it.
- 关灯 is a playback state, not an image. There is no black or white reset
  image; the display goes dark through the Now Playing renderer.
- The white-center placeholder is the receiver's fresh-session scene, not
  the cover and not the Sound Glow palette. The sender cannot set it, and
  clearing the artwork slot after a template does not bring it back. The app
  preview shows a neutral media-glyph tile for the same no-cover state.
- Cover priority needs the media cover. The receiver track-info push is
  always on; a coverless track keeps the previous light rather than showing
  white.
- The cover preview is local. The receiver runs its own Lab k-means and its
  own audio-driven Sound Glow; the preview mirrors the palette, not the live
  LED frames.
- The template carrier is not a picture. It is three flat color fields; the
  app preview paints the three colors the receiver extracts, not the carrier.
- The flyout list stays open after a template pick; hiding the flyout folds
  it, so the next open starts collapsed.
- The light must be on before the priority segment or a template changes. The
  off preview dims like the templates, and the off chip reads 已关灯.
- `TopDisplayCapable` accepts any `AudioAccessory*` at 27.2 or later,
  including 6,1; there is no model whitelist. An unknown model or version
  stays off.
- A stereo pair's simultaneous repaint is not yet verified; the image is
  skipped for a member that is not top-display capable, while the playback
  state still reaches every member.
- A state `1` on a stereo follower can leave its renderer white; a new
  session restores it. The sender avoids a redundant state `1` and resends
  the artwork after a resume's state `1`; if it sticks, reconnect.
