//go:build windows

package system

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	elevatedChildArg = "--wx-mini-video-elevated"
	terminalChildArg = "--wx-mini-video-terminal"
	configPathArg    = "--wx-mini-video-config="
)

type consoleEnvironment struct {
	executable, directory string
	elevated, inTerminal  bool
	findTerminal          func() (string, error)
	launch                func(executable string, args []string, elevate bool) error
}

// PrepareConsole runs before configuration, certificates or the proxy are loaded.
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
		elevated:   windows.GetCurrentProcessToken().IsElevated(),
		inTerminal: os.Getenv("WT_SESSION") != "",
		findTerminal: func() (string, error) {
			return exec.LookPath("wt.exe")
		},
		launch: func(executable string, args []string, elevate bool) error {
			return launchConsole(executable, args, directory, elevate)
		},
	}, args)
}

func prepareConsole(env consoleEnvironment, args []string) (bool, error) {
	var elevatedChild, terminalChild bool
	forwarded := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case elevatedChildArg:
			elevatedChild = true
		case terminalChildArg:
			terminalChild = true
		default:
			forwarded = append(forwarded, arg)
		}
	}
	if !env.elevated {
		if elevatedChild || terminalChild {
			return false, fmt.Errorf("重启后的进程没有管理员权限，请右键程序选择“以管理员身份运行”")
		}
		if err := env.launch(env.executable, append(forwarded, elevatedChildArg), true); err != nil {
			if errors.Is(err, windows.ERROR_CANCELLED) {
				return false, fmt.Errorf("已取消管理员权限请求，程序未启动: %w", err)
			}
			return false, fmt.Errorf("请求管理员权限失败: %w", err)
		}
		return true, nil
	}
	// UAC starts a new console even when WT_SESSION was inherited from the caller.
	if terminalChild || (env.inTerminal && !elevatedChild) {
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
	if err := env.launch(terminal, terminalArgs, false); err != nil {
		return false, fmt.Errorf("Windows Terminal 启动失败，请在管理员 Windows Terminal 中手动运行程序: %w", err)
	}
	return true, nil
}

func launchConsole(executable string, args []string, directory string, elevate bool) error {
	if !elevate {
		cmd := exec.Command(executable, args...)
		cmd.Dir = directory
		return cmd.Run()
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	parameters, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return err
	}
	cwd, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, parameters, cwd, windows.SW_SHOWNORMAL)
}

func ShowStartupError(err error) {
	text, _ := windows.UTF16PtrFromString(err.Error())
	caption, _ := windows.UTF16PtrFromString("wx-mini-video")
	_, _ = windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONERROR)
}
