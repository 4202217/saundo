# 更新日志

saundo 的显著变更记录在这里。格式遵循 Keep a Changelog，版本使用语义化版本。`mygo build` 读取 `CHANGELOG.md` 的 `## <version>` 段作为发布说明。本文件是中文翻译，版本标题与 `CHANGELOG.md` 保持一致。

## 0.4.1 - 2026-11-02

### 修复

- 音量读数现在显示 0% 到 100%，不再显示 0% 到 10000%。
- 丢帧只在串流时显示；空闲采集不再显示不断增长的丢帧数。
- 非 ASCII 发送端名称在 AP2 会话 plist 中改用 UTF-16BE 编码，Mac 接收端显示中文名称，不再乱码。
- 设置窗口的标题栏、侧栏和内容使用同一个背景。

## 0.4.0 - 2026-11-02

### 新增

- 英文和简体中文界面，带语言设置，默认跟随系统语言。托盘菜单、flyout、设置、对话框和屏幕阅读器标签都跟随所选语言。
- `cmd/saundo/locales` 下的类型化消息目录，由 mygo 生成，带 key 一致性测试和 `mygo i18n check` 门。
- 英文和伪语言（`en-XA`）渲染测试。

### 变更

- 界面文案来自目录；代码和诊断保持英文。
- 项目文档拆分为 architecture、protocol、devices、UI、implementation、verification。
- `make check` 现在校验目录和 mygo UI 规则。

### 修复

- 可访问性分支的窗口生命周期、键盘操作和空闲帧工作。
