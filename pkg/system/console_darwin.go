package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
)

func PrepareConsole() (bool, error) {
	if isatty.IsTerminal(os.Stdin.Fd()) {
		return false, nil
	}
	if os.Getenv("WX_MINI_VIDEO_MAC_TUI") != "" {
		return false, fmt.Errorf("未取得交互终端，请从 Terminal 打开程序")
	}
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	root := macPortableDirectory(exe)
	configPath := os.Getenv("WX_MINI_VIDEO_CONFIG")
	if configPath == "" {
		configPath = filepath.Join(root, "wx-mini-video.yaml")
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return false, err
	}
	if info, err := os.Stat(configPath); err != nil || !info.Mode().IsRegular() {
		return false, fmt.Errorf("无法找到解压目录中的 wx-mini-video.yaml。请先完整解压 ZIP；首次运行时请在 Finder 中将 wx-mini-video.app 拖出解压目录，再拖回该目录并重新双击。macOS 首次运行保护可能临时移动应用，不能从该临时目录保存媒体")
	}
	root = filepath.Dir(configPath)
	command := "cd " + ShellQuote(root) + " && /usr/bin/env WX_MINI_VIDEO_MAC_TUI=1 WX_MINI_VIDEO_CONFIG=" + ShellQuote(configPath) + " " + ShellQuote(exe)
	for _, arg := range os.Args[1:] {
		command += " " + ShellQuote(arg)
	}
	_, err = RunMacCommand("/usr/bin/osascript", "-e", "on run argv\n tell application \"Terminal\"\n activate\n do script (item 1 of argv)\n end tell\nend run", "--", command)
	if err != nil {
		return false, fmt.Errorf("打开 Terminal 失败，请在系统提示中允许本应用控制 Terminal: %w", err)
	}
	return true, nil
}

func macPortableDirectory(exe string) string {
	dir := filepath.Dir(exe)
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" && strings.HasSuffix(filepath.Base(filepath.Dir(filepath.Dir(dir))), ".app") {
		return filepath.Dir(filepath.Dir(filepath.Dir(dir)))
	}
	return dir
}

func ShowStartupError(err error) {
	fmt.Fprintln(os.Stderr, err)
	_, _ = RunMacCommand("/usr/bin/osascript", "-e", "on run argv\n display dialog (item 1 of argv) with title \"wx-mini-video\" buttons {\"好\"} default button 1 with icon stop\nend run", "--", err.Error())
}
