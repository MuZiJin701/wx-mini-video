//go:build preview

// Command uipreview 把真实 TUI View 渲染成 ANSI 文本，供 build/ansi_to_png.py
// 生成文档预览图。它不进入正式构建，运行方式：
//
//	go run -tags preview ./build/uipreview
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"wx_channel/internal/tui"
)

func main() {
	outDir := filepath.Join("build", "preview")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "创建预览目录失败:", err)
		os.Exit(1)
	}
	size := struct{ width, height int }{110, 40}
	for _, kind := range tui.PreviewKinds {
		text, err := tui.RenderPreview(kind, size.width, size.height)
		if err != nil {
			fmt.Fprintln(os.Stderr, kind, "渲染失败:", err)
			os.Exit(1)
		}
		path := filepath.Join(outDir, kind+".ansi")
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			os.Exit(1)
		}
		fmt.Println("已生成", path)
	}
}
