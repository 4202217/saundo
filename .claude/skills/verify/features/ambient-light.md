# Ambient light

The HomePod top display renders the current Now Playing cover. While the
ambient light is on, the sender puts a generated palette image there instead
of the media cover. One global choice, not per receiver.

## Sub-features

- 氛围灯 settings page and the flyout's ambient card. The settings page owns
  the 开启氛围灯 switch; the flyout card appears only while it is on and has
  no switch. The tray menu has no ambient item.
- Seven presets: 默认 (origin reset), 暖烬, 沙丘, 苔庭, 潮汐, 兰夜, 月白.
  `internal/lamp` renders six palettes plus origin as deterministic 600×600
  JPEGs and caches each one.
- The image leaves as one `SET_PARAMETER image/jpeg` cover on the encrypted
  control channel. It is sent when a preset changes and when a stream starts;
  the artwork POST command is not used.
- One cover slot. The light owns it while on. With the light off, the
  receiver track-info cover (在接收器上显示曲目信息) wins when it is on, and
  the origin image is the fallback. A media cover change updates the text
  without displacing the light.
- Capability gate. `device.TopDisplayCapable` accepts the HomePod family at
  software 27.2 or later and fails closed on an unknown model or version.
  A stereo group fans the image out to each capable member.
- Sound Glow must be enabled on the HomePod's own settings. Without it the
  top stays white.

## How to get to it (user POV)

设置 → 氛围灯, or the ambient card under the playback card in the flyout.
Turning the switch on shows the card; turning it off hides it.

## Driving it

Headless UI:

```bash
go test -run 'TestFlyoutLampCardVisibleWhenOn|TestFlyoutLampExpandsAndChoosesPreset|TestFlyoutLampOffHidesSection|TestSettingsLampSwitchHidesFlyout|TestSettingsLampPageChoosesPreset|TestSettingsLampPageOffShowsHintAndDisablesPresets|TestLampSurfacesInEnglish' -v ./internal/app/
```

Renderer and capability:

```bash
go test -run 'TestPresetsMatchCatalog|TestGeneratedJPEGsAre600Square|TestPresetPixelsFollowTheBlobs|TestGeneratorCachesPerPreset' -v ./internal/lamp/
go test -run 'TestTopDisplayCapable' -v ./internal/airplay/device/
```

Sender and engine:

```bash
go test -run 'TestSetLampArtworkSendsCover|TestSetLampArtworkWaitsForStreaming|TestSetLampArtworkDuplicateIsNoOp|TestSetLampArtworkDoesNotTouchDMAPText|TestSetLampArtworkClearSendsNothing' -v ./internal/airplay/sender/
go test -run 'TestLampToggleAppliesAndPersists|TestLampAppliedWhenStreamingStarts|TestLampUnknownPresetFallsBackToOrigin|TestLampSkipsIncapableReceivers|TestLampTogglingTwiceLogsTheSkipOnce|TestGroupArtworkFansOutToBothCapableMembers|TestGroupArtworkSkipsIncapableMember|TestMediaCoverWinsWhenLampIsOff|TestLampCoverWinsWhileOn' -v ./internal/engine/
```

Live, with a HomePod at software 27.2 or later and Sound Glow on:

1. Connect and stream, then open 设置 → 氛围灯.
2. Turn 开启氛围灯 on and choose 潮汐. The top display repaints within a
   second or two; the log shows a `SET_PARAMETER image/jpeg` cover.
3. Switch 沙丘 to 兰夜. Each repaint replaces the previous palette.
4. Turn the switch off. The top returns to its default white, or to the media
   cover while 在接收器上显示曲目信息 is on.
5. Disconnect and reconnect. The chosen preset reapplies when the stream
   starts.
6. Repeat with a stereo pair: each capable member repaints.

Proven end state: the top display shows the chosen preset, the change
survives a settings-window close/reopen and an app restart, and the receiver
keeps its own default or media cover while the light is off.

## Gotchas

- Sound Glow off leaves the top white. That is the HomePod setting, not the
  app; the app cannot enable it.
- A true LED-off has no AirPlay command. 默认 sends a black cover that the
  receiver lifts to its default white; do not expect a dark top.
- The light and the receiver track-info cover share one slot. The light wins
  while it is on; a song change must not replace it.
- The 6,1 and first-generation HomePod are outside `TopDisplayCapable` until
  they are verified. An unknown model or version stays off.
- A stereo pair's simultaneous repaint is not yet verified; a member that is
  not top-display capable is skipped.
- The flyout card is absent while the setting is off; that is not the same as
  no Now Playing card.
