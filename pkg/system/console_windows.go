//go:build windows

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	terminalChildArg = "--wx-mini-video-terminal"
	configPathArg    = "--wx-mini-video-config="
)

type consoleEnvironment struct {
	executable, directory string
	inTerminal            bool
	findTerminal          func() (string, error)
	launch                func(executable string, args []string, directory string) error
}

// PrepareConsole 在加载配置、证书和代理前选择 Windows Terminal。
// 主程序始终以当前用户运行；证书安装/旧证书清理由证书包在需要时单独请求 UAC。
func PrepareConsole() (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	directory, err := os.Getwd()
	if err != nil {
		return false, err
	}
	var args []string
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, configPathArg) {
			if err := os.Setenv("WX_MINI_VIDEO_CONFIG", strings.TrimPrefix(arg, configPathArg)); err != nil {
				return false, err
			}
			continue
		}
		args = append(args, arg)
	}
	if configPath := strings.TrimSpace(os.Getenv("WX_MINI_VIDEO_CONFIG")); configPath != "" {
		absolutePath, err := filepath.Abs(configPath)
		if err != nil {
			return false, err
		}
		args = append(args, configPathArg+absolutePath)
	}
	return prepareConsole(consoleEnvironment{
		executable: executable,
		directory:  directory,
		inTerminal: os.Getenv("WT_SESSION") != "",
		findTerminal: func() (string, error) {
			return exec.LookPath("wt.exe")
		},
		launch: launchConsole,
	}, args)
}

func prepareConsole(env consoleEnvironment, args []string) (bool, error) {
	terminalChild := false
	forwarded := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == terminalChildArg {
			terminalChild = true
			continue
		}
		forwarded = append(forwarded, arg)
	}
	if terminalChild || env.inTerminal {
		return false, nil
	}
	terminal, err := env.findTerminal()
	if err != nil {
		return false, nil
	}
	terminalArgs := []string{"-w", "new", "new-tab", "-d", env.directory, env.executable, terminalChildArg}
	terminalArgs = append(terminalArgs, forwarded...)
	// wt treats unescaped semicolons as separators between terminal commands.
	for i := range terminalArgs {
		terminalArgs[i] = strings.ReplaceAll(terminalArgs[i], ";", `\;`)
	}
	if err := env.launch(terminal, terminalArgs, env.directory); err != nil {
		return false, fmt.Errorf("Windows Terminal 启动失败，请在当前控制台或 Windows Terminal 中手动运行程序: %w", err)
	}
	return true, nil
}

func launchConsole(executable string, args []string, directory string) error {
	cmd := exec.Command(executable, args...)
	cmd.Dir = directory
	return cmd.Run()
}

func ShowStartupError(err error) {
	text, _ := windows.UTF16PtrFromString(err.Error())
	caption, _ := windows.UTF16PtrFromString("wx-mini-video")
	_, _ = windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}
