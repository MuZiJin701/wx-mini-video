package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ShellQuote keeps paths and network service names as single shell arguments.
func ShellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func RunMacCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func RunMacAdmin(commands [][]string) error {
	script, err := macAdminScript(commands)
	if err != nil || script == "" {
		return err
	}
	_, err = RunMacCommand("/usr/bin/osascript", "-e", "on run argv\n do shell script (item 1 of argv) with administrator privileges\nend run", "--", script)
	return err
}

func macAdminScript(commands [][]string) (string, error) {
	parts := make([]string, 0, len(commands))
	for _, command := range commands {
		if len(command) == 0 || (command[0] != "/usr/sbin/networksetup" && command[0] != "/usr/bin/security") {
			return "", fmt.Errorf("拒绝未识别的 macOS 管理命令")
		}
		args := make([]string, len(command))
		for i, value := range command {
			args[i] = ShellQuote(value)
		}
		// networksetup can report an error in stdout even when it exits successfully.
		parts = append(parts, "(output=$(LC_ALL=C "+strings.Join(args, " ")+" 2>&1); status=$?; if [ \"$status\" -ne 0 ]; then printf '%s\\n' \"$output\" >&2; exit \"$status\"; fi; case \"$output\" in *'** Error:'*) printf '%s\\n' \"$output\" >&2; exit 1;; esac)")
	}
	return strings.Join(parts, " && "), nil
}
