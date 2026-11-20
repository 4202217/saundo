<!-- reviewed-through: 2026-11-20 -->

# UI copy standard

Scope: every string in `cmd/saundo/locales/en.json` and `cmd/saundo/locales/zh-CN.json`, plus any string built in code that reaches a surface. This page is the rulebook for writing and reviewing that copy. The catalogs are the source of truth. When code and this page disagree, code wins and this page is updated in the same commit.

## One entity, one name

| Concept | Chinese | English | Do not use |
|---|---|---|---|
| AirPlay receiver | 接收器 | receiver | 接收设备, 接收端, 设备, 当前接收器 |
| The tray panel (flyout) | 托盘面板 | tray panel | 面板 (bare), 控制面板, 控制中心 |
| Playback | 播放 | play | 推流, 串流 |
| Capture | 采集 | capture | 录制 |
| The media card | 媒体卡片 | media card | 媒体卡 (short form) |
| Ambient light | 氛围灯 | ambient light | 顶屏灯光 as the feature name |
| The light's target | 顶屏 | top display | 屏幕 |
| A target's volume | 音量 | volume | 音量大小, 输出音量 |
| A target's stereo side | 声道 | channel | 左声道, 右声道, 单声道 |
| A receiver on the machine running saundo | 本机 | local | 自身接收器, this device, this Mac as the chip |

The receiver is the product concept. The flyout section head and the settings page use the same word. A Windows audio endpoint is named 系统声音 / system audio, never 接收器 or 设备. A receiver on this machine is never a target. `本机` / `Local` as a receiver label appears only on its row; the mute-local copy uses `本机输出` / `local output` for the computer's audio output.

The ambient light is turned off with 关闭氛围灯 and on with 开启氛围灯. Do not use 熄灯.

The tray menu's receiver rows keep the flyout's 离线 status. The session action row uses 取消连接, 断开连接, and 重试连接. The ambient submenu uses 关闭氛围灯 or 开启氛围灯, 封面, and the template names. The choices are disabled while the light is off; 开启氛围灯 turns it on first. While lit, the current choice is disabled, greyed, and its label ends in a single-color ✔; while off, the stored choice carries 上次 / Last used.

## Latency and stability

| Concept | Chinese | English | Do not use |
|---|---|---|---|
| The sender buffer the user chooses | 延迟模式 | latency mode | 调度缓冲, 缓冲延迟 |
| Session audio stability | 稳定性 | stability | 丢样本, 丢帧 |
| Where the latency mode is changed | 可在音频页调整 | Adjust on Audio | — |

The connection page shows no latency value; the latency mode lives on 音频 and the flyout's ALAC line carries the effective value. The receiver's own processing time is measured for diagnostics and has no surface.

## Per-device controls

A target is one receiver or one stereo pair. Volume and channel belong to the target. The settings receiver row keeps the volume slider and the channel segment; the per-device delay row was removed from settings at the user's request. In the flyout, each session member's row carries its own volume slider under the name and an icon-only channel button before it; the row's model line yields while the device is a member. A row the engine is joining or reopening is plain and its model line reads 正在连接… / Connecting… while joining, 正在重连… / Reconnecting… while an automatic retry reopens it. The joining row's 取消 / Cancel removes just that target; a member's 断开连接 / Disconnect removes only that row's target; the device section head's 全部断开 / Disconnect all ends the whole session. The slider's label names its target: `Volume · HomePods` / `音量 · HomePods`. The channel button has no visible text: its tooltip and accessible name read `Channel: {current} · Click for {next}` / `声道：{current} · 点击切换到 {next}`, and a stereo pair reads `Channel: {current} · A stereo pair always plays stereo` / `声道：{current} · 立体声对固定为立体声`. The button's solid side is the current side. The speaker glyph is a button with no visible label; its tooltip and accessible name read `Mute {device}` / `静音 {device}`, or `Unmute {device}` / `取消静音 {device}` at zero volume.

The channel choices are `Stereo` / `立体声`, `Left` / `左`, and `Right` / `右`. Left or right copies that input channel into both outputs. The settings row hints this with `Left or right copies that channel to both outputs` / `左或右会把该声道复制到两个输出`; as a row hint it carries no terminal punctuation. A stereo pair always plays stereo: its row shows a disabled `立体声`, never `左` or `右`.

A stereo combo (软件立体声组合) reads 立体声组合 / Stereo combo as its chip and section label, and names itself `{left} + {right}` from its two members. The create entry reads 组成立体声组合 / Create stereo combo with the hint 两台接收器各播一个声道 / One receiver per channel; the dialog commits as 组合 / Create, and choosing one receiver for both sides reads 两侧不能选择同一台接收器 / The two sides cannot be the same receiver. The layout row reads 声道布局 / Channel layout and the swap action 交换左右声道 / Swap left and right. Unlinking reads 解除组合 / Unlink; the dialog 解除立体声组合 / Unlink stereo combo states 「{name}」将拆回两台独立接收器。/ {name} splits back into two standalone receivers. and the hint 组合的音量不再适用；每台接收器保留自己的设置。/ The combo volume stops applying; each receiver keeps its own settings. An offline member warns 「{name}」未连接，无法启动组合 / {name} is offline, so the combo cannot start. The flyout's locked channel tooltip reads 立体声组合 · 左 + 右（已锁定） / Stereo combo · Left + Right (locked). The engine's refusal reads 这两台接收器无法组成立体声组合。/ These receivers cannot form a stereo combo.

A multi-target session names itself as `{name} + {name}` where the engine publishes one name (`ConnectedName`); the settings sidebar instead names it `{name} 和其他 {n} 台` / `{name} and {n} more` — the first member plus a count — because a joined name overflows the 190-DIP sidebar, and the joined form rides that row's tooltip. The connection card's subtitle reads `{n} 台接收器` / `{n} receivers`. The card's receiver-processing stat keeps `最高 {value}` / `Up to {value}` because the engine reports the slowest member.

A member that leaves while the rest continue reads `{name} 已断开，其他设备继续播放` / `{name} disconnected; the other devices keep playing`; a join in progress reads `正在加入 {name}，其他设备会短暂暂停` / `Adding {name}; the other devices pause briefly`. The notice's dismiss action is 忽略 / Dismiss, and its only other action is 重试 / Retry on the loss.

A refused saved password reads `{device} 的密码未被接收器接受。` / `The receiver did not accept the password for {device}.` The masked password dialog that opens is the same one used on first connect; the error explains why it reopened.

The 连接 page names its two sections 已连接设备 / Connected devices and 本次连接 / This connection, in the group-label style above the card. A device row's state reuses 正在播放 / Streaming, 正在连接… / Connecting…, 正在重连… / Reconnecting…, and 连接失败 / Connection failed. A device dropped while the rest continue appears as `{name} 已断开` / `{name} disconnected` with 重新连接 / Reconnect. The measured card reads 已连接时长 / Connected for, 稳定性 / Stability with `丢样 {drops} · 欠载 {underruns}` / `{drops} dropped · {underruns} underruns`, 网络重传 / Network retransmit, and 重连 / Reconnects; a healthy device's counters stay hidden, and a notable one reads `重传 {pct}%` / `{pct}% retransmitted` and `欠载 {n}` / `{n} underruns`.

## One state, one phrase

| State | Long form | Short form |
|---|---|---|
| Connecting | 正在连接… / Connecting… | 连接中 / Connecting |
| Reconnecting | 正在重连… / Reconnecting… | 连接中 / Connecting |
| Pairing | 正在配对… / Pairing… | — |
| Streaming | 正在通过 {name} 播放 / Streaming to {name} | 正在播放 / Streaming |
| Standby | 已待机 · 播放声音即唤醒 / Standby · wakes when sound plays | 待机 / Standby |
| Connected | 已连接 / Connected | 已连接 / Connected |
| Disconnected | 未连接 / Not connected | 未连接 / Not connected |
| Failed | 连接失败 / Connection failed | 连接失败 / Connection failed |
| Taken over | 已被其他设备接管 / Taken over by another device | 已被其他设备接管 / Taken over by another device |
| Update available | {version} 可用 / Version {version} is available | {version} 可用 / {version} available |
| Update downloading | 正在下载 {version} / Downloading {version} | 正在下载 {percent}% / Downloading {percent}% |
| Update ready | {version} 已下载 / {version} is ready | 待重启 / Update ready |
| Update installing | 重启后完成安装。 / Restart to finish installing. | — |

The long form goes in the footer, the sidebar, and a status line. The short form goes in a chip. Never invent a third wording for the same state.

The takeover state keeps the error phase. Its active-row action is 重新连接 / Reconnect.

## Tone

The interface speaks to the person using it, not about how it is built.

- Do not mention reference implementations, backoff schedules, pairing proofs, signatures, or handshakes. Say what the user experiences.
- A subtitle adds a fact. It never restates its title.
- A dialog subtitle names the fields or the effect. It never repeats the row hint that opened it.
- An action names its object. `检查更新` is an action. `立即检查` is a fragment.
- Use `设置` for a first-time value and `更改` for an existing one. Use `启用` and `关闭` for switches. Do not use `打开` for a switch.

## Units and format

- Latency uses one formatter, `settings.LatencyText`. Below 1000 ms it prints `700 ms`. At or above 1000 ms a whole number of seconds prints without a decimal, `2 s`; any other value prints one decimal, `1.5 s`. Slider scale endpoints follow the same rule.
- Volume prints as an integer percent.
- Sample rates print as `44.1 kHz` or `48 kHz`.
- Chinese copy uses full-width punctuation. Prose ends with `。` / `.`: page descriptions, card body text, notes, dialog copy, status lines, and empty states. A settings-row subtitle, a link description, and a chip label carry no terminal punctuation. English labels and menu items use sentence case, except product names.
- The ellipsis is the single character `…`, with no space before it.
- An IP address, a port, and a version number use their own digits and units. Do not localize them.

## Allowed technical terms

`ALAC`, `AirPlay`, `AirPlay 1`, `AirPlay 2`, `PTP`, `NTP`, `mDNS`, `DSP/DAC`, `IP`, `PIN`, `Windows`, `HomePod`, `Mac`.

Every other protocol or implementation term needs a plain-language replacement before it reaches a surface. `auth-setup`, `handshake`, `pairing proof`, and `backoff` are not allowed.

## Redundancy

- The row hint and the dialog subtitle explain different things. The row says why the feature exists. The dialog says what to fill in.
- A status line and its card title do not repeat the same sentence.
- A chip may shorten a state. It may not rename it.

## Notes

- A caveat about something outside the card sits under the card with an info glyph. It names the control or setting it concerns.
- An interaction hint or a description stays inside the card as plain secondary text, without the glyph.

## Review gates

- `TestCatalogHasNoDeadKeys` fails when a catalog key has no caller.
- `TestCatalogBannedTerms` fails when Chinese copy contains a banned synonym or an implementation term.
- `TestPerDeviceControlCopyBudgets` keeps the new labels within their one-line budgets.
- `TestLatencyText` pins the unit rule.

A new UI string lands with its key, both languages, the surface that shows it, and the gate that covers its rule. Deleting a surface deletes its keys in the same change.
