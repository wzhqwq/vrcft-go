# VRCFaceTracking-Go

简体中文 | [English](README.md)

VRCFaceTracking-Go 是一款 Windows 桌面应用，可通过 OSC 将本地插件提供的面部与眼部追踪数据发送给当前的 VRChat Avatar。应用提供插件输入、Avatar 参数、追踪调节和故障诊断的统一界面。

## 主要功能

- **跟随当前 Avatar。** 监听 VRChat 的 Avatar 切换，读取对应的本地 OSC 配置，只发送该 Avatar 支持的参数。也可以在“设置”中指定备用 Avatar 配置。
- **管理本地插件。** 发现和启用插件，查看插件能力、活动状态、帧率与错误。内置 Steam Link 插件通过本机 OSC 接收眼部和表情追踪数据。
- **调节追踪效果。** 为所有通道或指定通道设置输入校准、响应、平滑及信号丢失处理；互斥组可避免冲突通道同时驱动 Avatar。
- **连接 VRChat OSC。** 自动发现 OSC 输出目标，或手动指定 IP 地址与端口。
- **查看运行情况。** “概览”展示 Avatar 驱动状态、支持与正在驱动的参数、插件输入和 OSC 输出；“诊断”展示启动故障、模块状态和可复制的近期脱敏日志。

## 项目来源与致谢

VRCFaceTracking-Go 是对 [VRCFaceTracking](https://github.com/benaclejames/VRCFaceTracking) 的独立 Go 语言重新开发，并非 VRCFaceTracking 项目的官方版本，也不代表获得其认可。

部分内置插件的开发借鉴了社区项目。其中，Steam Link 插件的 OSC 字段映射参考了 Valve 的相关设置文档和 [LinkFT](https://github.com/ykeara/LinkFT)；该适配器自行实现映射，没有复用 LinkFT 源码。具体参考资料和适用范围见 [Steam Link 来源说明](plugins/steamlink/README.md#input-evidence-and-limits)。

## Windows 上手

1. 在 PowerShell 中运行 `./build/build-desktop.ps1` 构建桌面应用。生成的程序位于 `build/bin/vrcft-go2.exe`，内置 Steam Link 插件位于 `build/bin/plugins/steamlink/`。如已安装 NSIS，也可以运行 `./build/build-desktop.ps1 -NSIS` 制作安装包。产物目录详见[构建说明](build/README.md)。
2. 启动应用，在“设置”中检查 Avatar OSC 目录，必要时指定备用 Avatar 配置。保存后的“设置”修改会在重启应用后生效。
3. 如使用 Steam Link，请在 Steam Link / SteamVR 中开启眼部、面部追踪共享及 OSC 输出，将输出端口设为插件的 `listenPort`（默认 `9015`），然后在“插件”页启用 **Steam Link**。具体操作见 [Steam Link 设置说明](plugins/steamlink/README.md)。
4. 打开 VRChat，在“概览”查看当前 Avatar、插件活动状态和 OSC 目标。如果追踪数据没有到达 Avatar，可在“诊断”中检查启动状态与近期日志。

Steam Link 插件实现的是已有文档描述的 OSC 配置。此前连接 Pico 4 Pro 的 Steam Link 生产测试已确认眼动追踪数据正确；该测试未验证表情及其他追踪参数。自动化测试使用合成数据包。详见[插件的适用范围](plugins/steamlink/README.md#input-evidence-and-limits)。

## 诊断与日志

Windows 上的脱敏 JSON 行日志保存在 `%APPDATA%\vrcft-go\logs\`。“诊断”页展示近期记录；即使追踪后端未能启动，也会保留启动故障信息。报告问题时可使用“复制诊断信息”。

## 项目文档

开发、架构、包检查和当前证据详见[项目规范](docs/project/README.md)、[里程碑](docs/project/milestones.md)和[生成的项目状态](docs/project/status.md)。项目状态是生成的证据，可能晚于当前源码改动。
