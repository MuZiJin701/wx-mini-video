package main

import (
	"fmt"
	"os"

	"wx_channel/internal/app"
	"wx_channel/internal/config"
	"wx_channel/internal/tui"
)

var AppVer = "260614"
var Mode = "debug"

func main() {
	cfg := config.New(AppVer, Mode)
	if err := cfg.LoadConfig(); err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}
	runtime := app.NewRuntime(cfg)
	defer runtime.Stop()
	if err := tui.Run(runtime); err != nil {
		fmt.Printf("运行失败: %v\n", err)
		_ = runtime.Stop()
		os.Exit(1)
	}
}
