<!-- reviewed-through: 2026-11-13 -->

# UI copy standard

Scope: every string in `cmd/saundo/locales/en.json` and `cmd/saundo/locales/zh-CN.json`, plus any string built in code that reaches a surface. This page is the rulebook for writing and reviewing that copy. The catalogs are the source of truth. When code and this page disagree, code wins and this page is updated in the same commit.

## One entity, one name

| Concept | Chinese | English | Do not use |
|---|---|---|---|
| AirPlay receiver | 接收器 | receiver | 接收设备, 接收端, 设备, 当前接收器 (except 播放设备 for a Windows audio endpoint) |
| The tray panel (flyout) | 托盘面板 | tray panel | 面板 (bare), 控制面板, 控制中心 |
| Playback | 播放 | play | 推流, 串流 |
| Capture | 采集 | capture | 录制 |
| The media card | 媒体卡片 | media card | 媒体卡 (short form) |
| Ambient light | 氛围灯 | ambient light | 顶屏灯光 as the feature name |
| The light's target | 顶屏 | top display | 屏幕 |
| A receiver on the machine running saundo | 本机 | local | 自身接收器, this device, this Mac as the chip |

The receiver is the product concept. The flyout section head and the settings page use the same word. A Windows audio endpoint is the one place `播放设备` stays. A receiver on this machine is never a target. `本机` / `Local` as a receiver label appears only on its row; the mute-local copy uses `本机输出` / `local output` for the computer's audio output.

The ambient light is turned off with 关灯 or 关闭氛围灯. Do not use 熄灯.

## Latency and stability

| Concept | Chinese | English | Do not use |
|---|---|---|---|
| The sender buffer the user chooses | 延迟模式 | latency mode | 调度缓冲, 缓冲延迟 |
| The receiver's own reported processing time | 接收器处理 | receiver processing | 接收器输出, 设备输出 |
| Session audio stability | 稳定性 | stability | 丢样本, 丢帧 |
| Where the latency mode is changed | 可在音频页调整 | Adjust on Audio | — |

The two latency values measure different stages. Never draw them as one bar, never add them, and never present the receiver's value as a total.

## One state, one phrase

| State | Long form | Short form |
|---|---|---|
| Connecting | 正在连接… / Connecting… | 连接中 / Connecting |
| Reconnecting | 正在重连… / Reconnecting… | 重连中 / Reconnecting |
| Pairing | 正在配对… / Pairing… | 配对中 / Pairing |
| Streaming | 正在通过 {name} 播放 / Streaming to {name} | 正在播放 / Streaming |
| Standby | 已待机 · 播放声音即唤醒 / Standby · wakes when sound plays | 待机 / Standby |
| Connected | 已连接 / Connected | 已连接 / Connected |
| Disconnected | 未连接 / Not connected | 未连接 / Not connected |
| Failed | 连接失败 / Connection failed | 连接失败 / Connection failed |
| Taken over | 已被其他设备接管 / Taken over by another device | 已被其他设备接管 / Taken over by another device |
| Update available | {version} 可用 / Version {version} is available | {version} 可用 / {version} available |
| Update downloading | 正在下载 {version} / Downloading {version} | 正在下载 {percent}% / Downloading {percent}% |
| Update ready | {version} 已下载 / {version} is ready | 待重启 / Update ready |
| Update installing | 重启后完成安装。 / Restart to finish installing. | 重启以完成安装 / Restart to install |

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
- `TestLatencyText` pins the unit rule.

A new UI string lands with its key, both languages, the surface that shows it, and the gate that covers its rule. Deleting a surface deletes its keys in the same change.
