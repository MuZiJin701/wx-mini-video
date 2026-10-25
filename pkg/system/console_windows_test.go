//go:build windows

package system

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConsoleStartupEntersTerminalOnce(t *testing.T) {
	executable := `D:\便携;目录 & 视频\wx-mini-video.exe`
	directory := `D:\工作;目录\`
	args := []string{configPathArg + `D:\配置;文件\settings.yaml`, "quoted\"value"}
	var launchedExe string
	var launchedArgs []string
	launches := 0
	env := consoleEnvironment{
		executable: executable, directory: directory,
		findTerminal: func() (string, error) { return `C:\WindowsApps\wt.exe`, nil },
		launch: func(exe string, args []string, dir string) error {
			launches++
			launchedExe, launchedArgs = exe, append([]string(nil), args...)
			if dir != directory {
				t.Fatalf("Terminal directory = %q, want %q", dir, directory)
			}
			return nil
		},
	}
	relaunched, err := prepareConsole(env, args)
	if err != nil || !relaunched || launchedExe != `C:\WindowsApps\wt.exe` {
		t.Fatalf("startup did not enter Terminal: relaunched=%v, err=%v", relaunched, err)
	}
	want := []string{"-w", "new", "new-tab", "-d", `D:\工作\;目录\`, `D:\便携\;目录 & 视频\wx-mini-video.exe`, terminalChildArg,
		`--wx-mini-video-config=D:\配置\;文件\settings.yaml`, "quoted\"value"}
	if !reflect.DeepEqual(launchedArgs, want) {
		t.Fatalf("Terminal launch changed directory or arguments: %#v", launchedArgs)
	}
	// Terminal 子进程不应再次启动终端。
	relaunched, err = prepareConsole(env, launchedArgs[6:])
	if err != nil || relaunched || launches != 1 {
		t.Fatalf("Terminal child restarted again: launches=%d, err=%v", launches, err)
	}
}

func TestConsoleStartupAlreadyInTerminal(t *testing.T) {
	env := consoleEnvironment{inTerminal: true}
	if relaunched, err := prepareConsole(env, nil); err != nil || relaunched {
		t.Fatalf("existing Terminal should be reused: %v, %v", relaunched, err)
	}
}

func TestConsoleStartupMissingTerminalKeepsConsole(t *testing.T) {
	env := consoleEnvironment{
		findTerminal: func() (string, error) { return "", os.ErrNotExist },
	}
	if relaunched, err := prepareConsole(env, nil); err != nil || relaunched {
		t.Fatalf("missing Terminal should keep the current console: %v, %v", relaunched, err)
	}
}

func TestConsoleStartupTerminalFailureDoesNotLoop(t *testing.T) {
	env := consoleEnvironment{
		findTerminal: func() (string, error) { return "wt.exe", nil },
		launch:       func(string, []string, string) error { return os.ErrPermission },
	}
	if relaunched, err := prepareConsole(env, nil); relaunched || err == nil {
		t.Fatalf("failed Terminal launch must report failure: %v, %v", relaunched, err)
	}
}

func TestConsoleLaunchPreservesWindowsArgumentsAndDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "中文 & directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	// Copy the test helper so the executable path also contains spaces and Chinese.
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	helperPath := filepath.Join(directory, "console helper.exe")
	helper, err := os.Create(helperPath)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(helper, source)
	closeErr := helper.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy helper: %v, %v", copyErr, closeErr)
	}
	resultPath := filepath.Join(directory, "result.json")
	t.Setenv("WX_CONSOLE_TEST_RESULT", resultPath)
	args := []string{"中文路径 with spaces", `quote"value`, `D:\tail\`, "&echo", ""}
	commandArgs := append([]string{"-test.run=^TestConsoleLaunchHelper$", "--"}, args...)
	if err := launchConsole(helperPath, commandArgs, directory); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Arguments []string
		Directory string
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Arguments, args) || result.Directory != directory {
		t.Fatalf("Windows launch changed arguments or directory: %#v", result)
	}
}

func TestConsoleLaunchHelper(t *testing.T) {
	resultPath := os.Getenv("WX_CONSOLE_TEST_RESULT")
	if resultPath == "" {
		return
	}
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct {
		Arguments []string
		Directory string
	}{os.Args[3:], directory})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
