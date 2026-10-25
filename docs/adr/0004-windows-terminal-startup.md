# Windows 启动优先使用 Terminal 并请求管理员权限

状态：已由 [ADR 0009](0009-user-scoped-windows-runtime.md) 修订（2026-10-25）：Terminal 首选保留，整体提权取消。以下保留原始决策背景。

用户已确认同一程序在传统控制台中中文缺失，而在 Windows Terminal 中显示正常。保留单二进制便携包，由程序启动入口选择已安装的 Windows Terminal，减少用户手动更改终端、字体和启动方式的步骤。

Windows 启动先检查实际进程令牌；没有管理员权限时通过 `ShellExecuteW` 的 `runas` 请求 UAC，同意后再优先进入新的 Windows Terminal 窗口。已经在管理员 Terminal 中运行时直接使用当前窗口。内部重启标记阻止重复提权或终端重启；标记不能代替实际管理员权限检查。取消和启动失败在配置、证书、代理初始化前停止，并显示 Unicode 错误对话框。

找不到 `wt.exe` 时保留现有控制台，不安装 Terminal，不修改系统默认终端。重启保留工作目录、参数及显式配置文件路径；不通过 shell 拼接执行命令，并转义 Terminal 的分号命令分隔符。自动化检查隔离 UAC 和 Terminal 开窗，仅验证启动编排与实际 Windows 子进程的路径、参数传递；完整 UAC 和 Terminal 启动仍需人工验收。
