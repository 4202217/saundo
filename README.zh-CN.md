<div align="center">

<a href="https://github.com/4202217/saundo"><img src="resources/icon.png" width="120" alt="saundo 图标：圆角深色底上的笑脸"></a>

# saundo · 声渡

### 把 Mac 或 PC 的全部系统声音串流到 AirPlay 2 接收器——托盘应用，纯 Go。

**声渡——把声音渡过去：声，声音；渡，摆渡、渡过。**<br>
**saundo** 取自サウンド，英语 sound 的日语片假名转写。<br>
HomePod、HomePod mini、Mac 和 Apple TV，这台 Mac 或电脑的声音，托盘一点就过去。

[![Listed on Awesome MyGo](https://awesomemygo.dev/badge.svg)](https://awesomemygo.dev/apps/saundo)

**[下载](https://github.com/4202217/saundo/releases/latest)** &nbsp;·&nbsp; **[问题反馈](https://github.com/4202217/saundo/issues)** &nbsp;·&nbsp; **[文档](docs/verification.md)** &nbsp;·&nbsp; [English](README.md) · **简体中文**

<br>

<p align="center">
  <img src="docs/screenshots/flyout-stream-light-zh.png" width="290" alt="声渡托盘面板：接收器列表、音量、媒体卡片">
  &nbsp;&nbsp;&nbsp;
  <img src="docs/screenshots/settings-connection-light-zh.png" width="470" alt="设置窗口：连接页">
</p>

</div>

<br>

## saundo 适合你吗？

saundo 采集系统正在播放的一切声音——Windows 上为 WASAPI 环回，macOS 上为 Core Audio 进程 Tap——并以未压缩 ALAC 经 AirPlay 2 串流到 HomePod、HomePod mini、Mac 接收器与 Apple TV。一个托盘图标，一个二进制，无 WebView：

- 你在 Mac 或 PC 上播放音频，想让它出现在同一网络里的 HomePod 上。
- 你要的是**全部**系统声音——浏览器、游戏、播放器——而不是单个应用的串流。
- 你偏好原生托盘应用，而不是 WebView。

如果你需要视频或屏幕镜像（本项目仅音频），或者你的接收器需要 Apple FairPlay——部分电视如此——那它不适合你。saundo 会在发现阶段识别这类设备并快速失败，而不是默默连上却没有声音。

支持 Windows 10 和 11 与 macOS 14.4+（Apple Silicon）；界面中英双语，默认跟随系统语言。macOS 构建以未签名 dmg 分发：首次启动需经 系统设置 → 隐私与安全性 → 仍要打开，详见 [docs/platforms.md](docs/platforms.md)。

## 快速开始

**1 · 安装**。从 **[Releases](https://github.com/4202217/saundo/releases/latest)** 下载安装器，或者从源码构建：

```bash
mingw32-make check  # gofmt 门、go vet、mygo UI vet、go test —— 验收门
mingw32-make build  # 经 mygo 构建 bin/saundo.exe 与 NSIS 安装器（bin/saundo-setup-*.exe），随后重启正在运行的应用
```

<sub>安装器尚未代码签名，SmartScreen 会提示。从源码构建需要 Go 1.27+（[mygo](https://github.com/egoist/mygo) 所需的工具链）和 MinGW-w64 的 `mingw32-make`；构建后运行 `bin/saundo-setup-*.exe`。macOS 上用 `make check` 与 `make build` 跑同一套门禁并生成 `bin/saundo.app`；Now Playing 适配器由 cmake 构建。</sub>

**2 · 连接**。启动后应用常驻托盘：左键打开面板，右键打开菜单。选择接收器并连接；需要密码的接收器只询问一次，之后记住。

**3 · 播放**。在 Mac 或电脑上随便播点什么。声音跟过去，音量滑块直接控制接收器自己的音量。

<sub>首次启动 Windows 防火墙会弹窗——请在专用网络上放行：发现（mDNS）和 PTP 时钟同步都需要入站 UDP。macOS 上请在 隐私与安全性 → 本地网络 中放行应用，原因相同。</sub>

## 它做什么

- **系统音频采集**——Windows 上对默认输出、指定端点或单个应用做 WASAPI 环回；macOS 上为 Core Audio 进程 Tap。
- **时序正确的 AirPlay 2**——44.1 kHz 未压缩 ALAC，PTP 锁定到接收器时钟（立体声组由发送端担任 PTP master），加密的控制与事件通道，按需重传。
- **HomePod 立体声组**——两个成员合并为一个目标，同步起播。
- **接收器管理**——mDNS 发现不到的设备可手动填写 IP 或主机名加入；发现过的接收器断电后仍保留；同时只标记一台自动连接；可把行从面板隐藏；每台接收器可单独设置 DSP/DAC 延迟和经系统密钥库保存的密码（Windows 为 DPAPI，macOS 为钥匙串）。定向单播会找回不响应组播的接收器；两个界面都只列出当前可用的接收器。
- **媒体卡片**——托盘面板显示系统媒体会话的应用名与图标、封面、标题、艺术家、会话提供的播放控制和五段实时电平表；可在打开的媒体会话之间切换。设置里的“正在播放”页控制媒体卡片、封面模式、封面配色和音频电平；接收器曲目信息推送始终开启。
- **氛围灯**——默认开启。托盘面板卡片上的电源动作控制 HomePod 顶屏开关；封面/模板优先分段选择跟随媒体封面（默认）或六套自定义三色模板。预览用取色后的三色光效，无封面时用媒体图标占位；HomePod 自己在全新会话仍显示白中心占位。暂停时保持当前灯光。关灯后优先分段和模板禁用，必须先开灯才能调整。可在设置的“氛围灯”页或托盘面板的氛围灯卡片选择。
- **连接后静音本机输出**——“连接”页的开关默认开启，在串流时静音本机渲染端点，断开后恢复；崩溃后由守卫文件恢复状态。
- **外观**——跟随系统、浅色或深色。macOS 上托盘面板可选系统气泡或圆角样式。
- **延迟模式**——低延迟 200 ms、标准 1.5 s、稳定 2 s，或 20 到 2000 ms 的自定义值。可在托盘面板或音频页切换。直播中切换会重启串流并显示加载状态。
- **托盘状态动画**——连接和串流时，微笑、声波、连接脸以 12 帧、每帧 100 ms 播放。设置里可关闭。Reduce Motion 时保持静态。
- **应用内更新（Windows）**——设置里的"更新"页显示可用版本、说明和发布历史。它安装签名更新。托盘面板的更新提示和托盘 tooltip 指向它。macOS 构建以未签名 dmg 分发，更新器保持关闭，直到有签名的 mac 更新源。
- **中英双语**——界面默认跟随系统语言，可在设置中切换，更新页和说明弹窗也随语言显示。
- **全类型配对**——HomePod 免密瞬态配对、Apple TV 屏幕配对码、接收器密码（首次连接对话框）；凭据经系统密钥库加密存储（Windows 为 DPAPI，macOS 为钥匙串）。
- **不打扰**——数字静音 2 秒后挂起串流并干净恢复；断线按退避自动重连；跟随接收器上的音量变化。

## 接收器支持

| 接收器 | 状态 |
|---|---|
| HomePod / HomePod mini（software 27） | 已验证 —— PTP 时序会话 |
| HomePod 立体声组 | 已验证 —— 发送端担任 PTP master |
| Mac（AirPlay 接收器，含"需要密码"） | 已验证 —— 密码配对 |
| Apple TV，屏幕配对码 | 已实现；硬件实测待做 |
| 需要 Apple FairPlay 的接收器（部分电视） | 不支持 —— 发现阶段识别，快速失败 |

每类设备如何广播、配对与表现：[docs/devices.md](docs/devices.md)。失败模式表（已连接但无声、配对 403/470、防火墙）与真机清单：[docs/verification.md](docs/verification.md)。

## 它如何组织

托盘面板与 UI 无关的引擎对话，引擎驱动无 I/O 的发送端状态机，走加密 RTSP。Windows 的 WASAPI 环回或 macOS 的 Core Audio 进程 Tap 喂入重采样器与环形缓冲；PTP follower（立体声组则为 master）让每一路流都锁定接收器时钟。设计决策与风险：[docs/architecture.md](docs/architecture.md)。线级协议：[docs/protocol.md](docs/protocol.md)。代码地图与平台接缝：[docs/implementation.md](docs/implementation.md) 与 [docs/platforms.md](docs/platforms.md)。

## 文档

- [docs/architecture.md](docs/architecture.md) —— 设计决策、技术选型、风险
- [docs/protocol.md](docs/protocol.md) —— 线级协议：配对、控制通道、AP2 会话、RTP、时序
- [docs/devices.md](docs/devices.md) —— 每类接收器的档案：如何广播、配对、表现
- [docs/ui.md](docs/ui.md) —— UI 设计语言：令牌、控件、动效、窗口
- [docs/implementation.md](docs/implementation.md) —— 代码地图：运行时模型、包、流程、构建
- [docs/platforms.md](docs/platforms.md) —— Windows/macOS 接缝、功能对照、打包
- [docs/verification.md](docs/verification.md) —— 自动化检查、真机测试配方、证据日志
- [docs/releasing.md](docs/releasing.md) —— 发版清单、签名密钥与回滚
- [SECURITY.md](SECURITY.md) —— 漏洞报告
- [AGENTS.md](AGENTS.md) —— 编码代理的操作指南

## 参考

- [airplay2-sender-cpp](https://github.com/akustikrausch/airplay2-sender-cpp) —— 主要蓝图（Apache-2.0）
- [pyatv](https://github.com/postlund/pyatv) —— RAOP 传输语义（MIT）
- [AirFlash](https://github.com/Ding-Kyoma/AirFlash) —— WASAPI 与 PTP 代码参考，按 GPLv3 选项移植
- [magpie](https://github.com/yetone/magpie) —— 托盘面板与设置窗口设计参考

## 许可证

saundo 是自由软件，采用 GPL-3.0-only。许可证正文见 [LICENSE](LICENSE)，第三方署名见 [NOTICE](NOTICE)。
