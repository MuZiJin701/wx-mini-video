package main

import (
	"errors"
	"fmt"
	"os"

	"wx_channel/internal/app"
	"wx_channel/internal/config"
	"wx_channel/internal/tui"
	"wx_channel/pkg/system"
)

var AppVer = "v0.1.9"
var Mode = "debug"

func main() {
	relaunched, err := system.PrepareConsole()
	if err != nil {
		system.ShowStartupError(err)
		os.Exit(1)
	}
	if relaunched {
		return
	}
	cfg := config.New(AppVer, Mode)
	if err := cfg.LoadConfig(); err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}
	runtime := app.NewRuntime(cfg)
	defer func() {
		if err := runtime.Stop(); err != nil {
			system.ShowStartupError(fmt.Errorf("恢复系统代理或关闭代理失败: %w", err))
		}
	}()
	if err := tui.Run(runtime); err != nil {
		system.ShowStartupError(errors.Join(err, runtime.Stop()))
		os.Exit(1)
	}
}
