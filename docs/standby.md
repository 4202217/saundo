<!-- status: implemented; live dark and resume verified 2026-11-10; long soak pending -->

# Idle standby

When saundo stays connected to a receiver but is not pushing RTP for a long time, the receiver should look idle, and the first audible sample should restore the stream without a lost start.

Only HomePod and HomePod mini at software 27.2 or later get a visible standby in this version. Every other receiver keeps its current behavior.

## What a receiver can do

Two levers exist. This repository has measured the first one only.

- **Dark.** `POST /command` with `updateMRPlaybackState 2` clears the receiver's Now Playing renderer and turns the HomePod top display dark while the control channel, event channel, and RTP session stay open. State 1 restores the current image. Measured on a HomePod mini `AudioAccessory5,1` 27.2 on 2026-11-07, with RTP flowing (protocol.md §10).
- **Disconnect.** TEARDOWN ends the session. A later start pays pair-verify, session SETUP, event channel, PTP readiness, and stream SETUP again. AirFlash's `standby.rs` closes the member sessions on silence and rebuilds the stream on new audio. Its default is off, 10 seconds, configurable 5 to 300 seconds.

There is no sleep command in the documented protocol. A HomePod's visible idle is its dark top display.

## Capability matrix

| Receiver | Dark standby | Behavior in this version |
|---|---|---|
| HomePod / mini, OS 27.2 or later | yes | dark after the idle threshold |
| HomePod / mini, OS below 27.2 or unknown | unknown | fail closed, no command, session stays lit |
| Stereo pair | per member | both capable members dark; the group filters |
| Mac AirPlay receiver | no | no command; session unchanged |
| Apple TV | no | no command; session unchanged |
| AirPlay 1, AirPort Express | no AP2 command channel | session unchanged |

The gate is the existing `device.TopDisplayCapable`: model prefix `AudioAccessory` and OS 27.2 or later. A missing model or version fails closed.

## Idle phases

The engine replaces the `audioPaused` boolean with one phase enum. A future disconnect standby adds `idleDetached`; it is not part of this version.

```go
type idlePhase uint8

const (
	idleActive    idlePhase = iota // pushing RTP, or a session that just started
	idleSuspended                  // not pushing, session alive
	idleDark                       // lamp off, session alive
)
```

```go
func (e *Engine) idlePhase(now time.Time) idlePhase
```

Derivation, in order:

1. No streaming transport, an AirPlay 1 session, or `pauseUnsupported`: `idleActive`.
2. The transport is pushing RTP: `idleActive`.
3. Dark eligible and `now - darkBase >= standbyAfter`: `idleDark`.
4. Otherwise: `idleSuspended`.

Dark eligible means AP2, `TopDisplayCapable`, `Lamp.Off` false, standby enabled, and no manual hold.

The 5 second silence policy is separate from the phase. While the transport pushes, `updateIdlePhase` stops the pacer after 5 seconds of digital silence and records the last push; the phase then follows the actual push state. The first audible block re-arms the pacer and updates the last push.

Transitions:

- active to suspended: the silence policy has called `SetPaused(true)`; the silence meter drops silent blocks.
- suspended to dark: `SetPlaybackState(false)`; one `updateMRPlaybackState` command with state 2.
- dark to active: the silence policy re-arms the pacer on the first audible block; the transition sends `SetPlaybackState(true)`, which restores the image and resends the cover, before the sender's next tick sends audio.
- suspended to active: the silence policy calls `SetPaused(false)`.
- any to active on session close: reset the phase and clear the hold.
- dark to suspended when standby becomes ineligible: `SetPlaybackState(true)`, then follow the suspended rules.

## Timing

`standbyAfter` is a setting. The default is 30 minutes. Stored values normalize to 1, 5, 10, or 30 minutes, and an unknown value becomes 30. The 5 second RTP suspend stays a constant.

The dark baseline is `max(last RTP push, sessionBaseline)`. The engine records the last push while the transport is pushing, so the threshold starts when the stream actually stopped, not when the last audible sample arrived. `sessionBaseline` is set when the session starts:

- a user connect sets it to the session start, so a fresh connect shows the light and darkens only after a full idle period;
- an automatic reconnect sets it to zero, so a background reconnect while the PC has not pushed for hours goes dark immediately and does not light the room.

AirFlash counts from the last audio sample; saundo counts from the last RTP packet, which is the state the receiver sees.

## Manual lamp action

Any user `SetLamp` call while the session is idle lifts the dark phase and suppresses it until the next audible audio. This covers turning the light on and changing its mode or template. The hold is per session and is not written to settings. A manual 关灯 stays authoritative; dark standby is a no-op while `Lamp.Off` is true.

## Settings

```json
"standby": { "enabled": true, "idleMinutes": 30 }
```

`internal/settings.Settings` gains `Standby StandbySettings`. `Standby` joins `engineOwnedFields`, and the app writes it through the engine like `Lamp`. `Default` sets enabled true and 30 minutes. Normalization accepts 1, 5, 10, and 30.

## UI

- The lamp settings page gets one row at the bottom of the card, after the templates. It reads 「自动关闭氛围灯」 and carries one 150×28 select with 从不, 1, 5, 10, and 30 分钟. The default is 30 分钟. Selecting 从不 is the off state; there is no separate switch.
- The UI maps that single select to the settings pair. 从不 sets `Enabled` false, and a minute value sets `Enabled` true with that `IdleMinutes`.
- The flyout lamp card shows 「待机」 and its power action reads 「开灯」 while dark. Pressing it holds the light on until the next audio.
- The device row or footer shows 「已待机 · 播放声音即唤醒」 while dark.
- `Snapshot` gains `Standby bool` for the live phase and `StandbyEnabled` and `StandbyMinutes` for the setting. `AudioIdle` keeps its current meaning for the tray face.
- New copy goes through `cmd/saundo/locales` in both languages.
- The settings row, the flyout label, and the power state get a mockup and approval before production UI code. The working mockup is `.audit/mockups/lamp-standby/mockup.html`.

## Engine and sender changes

- `internal/engine`: phase enum, push-state derivation, last-push tracking, threshold, hold, session baseline, snapshot fields, `SetStandby`, log lines.
- `internal/airplay/sender`: adds `Pushing() bool`; no wire change. Reuse `SetPaused` and `SetPlaybackState`. `SetPlaybackState(true)` already restores the artwork after a pause transition.
- `internal/settings`: struct, defaults, normalization, ownership.
- `internal/app`: settings row, flyout status and power state, locale strings.
- The capture ring grows to eight seconds at 48 kHz stereo, so the 5 second silence policy fits without counting silence as capture overflow.
- No new sockets, timers, or goroutines. The existing 25 ms audio poll while suspended also drives the threshold.

At stream start, compute the phase before `applyLamp`. An automatic reconnect that is already past the threshold goes straight to dark, without a state 1 flash.

## Failure and reconnect

- A clean control close during streaming stays a takeover. The engine clears standby and does not reconnect.
- A session that dies while dark follows the existing reconnect policy. The new session reapplies the phase at stream start.
- A user disconnect clears the phase and stays disconnected.
- PC sleep is not detected in this version. The session usually dies, and the reconnect policy handles the wake. No platform power notification is added.

## Tests

- Phase derivation with a fake clock: 5 second suspend, threshold dark, loud resume, hold, session baseline, ineligible devices.
- The last-push anchor advances while the pacer pushes and freezes when it stops; the threshold counts from the stop, not from the last audible sample.
- Exactly one playback-state command per transition; no duplicates and no redundant state 1.
- Stream start while already idle goes straight to dark.
- Stereo pair: both capable members receive the state; a non-capable member is filtered.
- Settings normalization and defaults.
- Snapshot and locale coverage for the new labels.

## Live verification

The 2026-11-07 measurement ran with RTP flowing. On 2026-11-10 a stereo pair went dark at both 1 minute and 5 minutes after the last push while the session stayed up, and the first sound sent state 1, restored both displays, and resumed the audio (see `docs/verification.md`, 2026-11-10). A two hour idle soak and the 30 minute default on a live pair remain.

## Out of scope

- Disconnect standby (the future `idleDetached` phase). It is the only lever for non-HomePod receivers and would need a reconnect-on-audio policy, a Now Playing pre-connect, and a busy-receiver probe.
- PC sleep and wake notifications.
- Using the Now Playing play state as a dark trigger.
