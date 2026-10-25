package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/miniprogram"
	"wx_channel/internal/tui/ui"
)

// hoverState 描述一个需要验证悬停稳定性的界面状态。
type hoverState struct {
	name  string
	build func(t *testing.T) model
}

func hoverStates() []hoverState {
	return []hoverState{
		{"目标选择区-启动中", func(t *testing.T) model {
			m := testModel(t)
			m.starting = true
			return m
		}},
		{"目标选择区-已发现", func(t *testing.T) model {
			m := testModel(t)
			m.started = true
			now := time.Now()
			m.programs = []miniprogram.Program{
				{AppID: "wx6ab29f61bd262656", LastSeen: now},
				{AppID: "wx1d2e3f4051627384", LastSeen: now.Add(-time.Minute)},
			}
			m.selectedProgram = m.programs[0].AppID
			m.focus = focusPrograms
			return m
		}},
		{"目标选择区-切换目标", func(t *testing.T) model {
			m := testModel(t)
			m.targetReady = true
			m.started = true
			m.programs = []miniprogram.Program{{AppID: "wx123", LastSeen: time.Now()}}
			m.selectedProgram = "wx123"
			m.appIDInput = "wx123"
			m.inputCursor = len([]rune("wx123"))
			return m
		}},
		{"媒体列表", func(t *testing.T) model {
			m := mediaTestModel(t)
			m.candidates = testCandidates(6)
			m.selected = 2
			m.selectedID = m.candidates[2].ID
			return m
		}},
		{"媒体列表-下载中", func(t *testing.T) model {
			m := mediaTestModel(t)
			m.candidates = testCandidates(4)
			m.downloading = true
			m.downloadID = m.candidates[0].ID
			m.downloadLive = true
			return m
		}},
		{"详情抽屉", func(t *testing.T) model {
			m := mediaTestModel(t)
			m.candidates = testCandidates(4)
			m.selected = 1
			m.selectedID = m.candidates[1].ID
			m.showDetails = true
			return m
		}},
		{"外观下拉", func(t *testing.T) model {
			m := mediaTestModel(t)
			m.candidates = testCandidates(2)
			m.appearanceOpen = true
			m.appearanceIdx = 1
			return m
		}},
		{"日志浮层", func(t *testing.T) model {
			m := mediaTestModel(t)
			m.candidates = testCandidates(2)
			m.logs = []string{"13:00:01 自检通过", "13:00:02 代理已启动", "13:00:03 已发现小程序"}
			m.showLogs = true
			return m
		}},
		{"退出确认", func(t *testing.T) model {
			m := mediaTestModel(t)
			m.downloading = true
			m.confirmQuit = true
			return m
		}},
		{"旧证书确认", func(t *testing.T) model {
			m := testModel(t)
			m.legacyPrompt = true
			return m
		}},
		{"代理恢复失败", func(t *testing.T) model {
			m := testModel(t)
			m.stopErr = errors.New("恢复系统代理失败")
			return m
		}},
		{"启动失败", func(t *testing.T) model {
			m := testModel(t)
			m.startErr = errors.New("certificate trust failed")
			return m
		}},
	}
}

func TestHoverNeverChangesPhysicalLayout(t *testing.T) {
	sizes := []struct{ width, height int }{{110, 40}, {80, 24}, {60, 20}}
	for _, state := range hoverStates() {
		for _, size := range sizes {
			m := state.build(t)
			m.width, m.height = size.width, size.height
			base := m.View()
			baseLines := strings.Split(base, "\n")
			assertPhysicalLines(t, size.width, size.height, baseLines, state.name)
			baseRegions := append([]hitRegion(nil), m.hits.regions...)
			if len(baseRegions) == 0 {
				t.Fatalf("%s %dx%d: no hit regions rendered", state.name, size.width, size.height)
			}
			for _, region := range baseRegions {
				x := region.x + region.w/2
				y := region.y + region.h/2
				next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: x, Y: y})
				hovered := next.(model)
				lines := strings.Split(hovered.View(), "\n")
				assertPhysicalLines(t, size.width, size.height, lines, state.name+" 悬停")
				if len(hovered.hits.regions) != len(baseRegions) {
					t.Fatalf("%s %dx%d: hover over %v changed hit region count: %d != %d",
						state.name, size.width, size.height, region.action, len(hovered.hits.regions), len(baseRegions))
				}
				for i, want := range baseRegions {
					got := hovered.hits.regions[i]
					if got != want {
						t.Fatalf("%s %dx%d: hover over %v moved hit region %d: %#v != %#v",
							state.name, size.width, size.height, region.action, i, got, want)
					}
				}
			}
			// 移出所有命中区域后，渲染必须与初始状态完全一致。
			emptyX, emptyY := findEmptyCell(baseRegions, size.width, size.height)
			next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: emptyX, Y: emptyY})
			cleared := next.(model)
			if got := cleared.View(); got != base {
				t.Fatalf("%s %dx%d: leaving all hit regions changed the frame", state.name, size.width, size.height)
			}
		}
	}
}

func assertPhysicalLines(t *testing.T, width, height int, lines []string, label string) {
	t.Helper()
	if len(lines) != height {
		t.Fatalf("%s: physical lines = %d, want %d", label, len(lines), height)
	}
	for index, line := range lines {
		if got := ui.Width(line); got != width {
			t.Fatalf("%s: line %d display width = %d, want %d: %q", label, index, got, width, line)
		}
	}
}

// findEmptyCell 找到一个不属于任何命中区域的坐标，用于验证悬停移出。
func findEmptyCell(regions []hitRegion, width, height int) (int, int) {
	for y := height - 1; y >= 0; y-- {
		for x := 0; x < width; x++ {
			hit := false
			for _, region := range regions {
				if region.contains(x, y) {
					hit = true
					break
				}
			}
			if !hit {
				return x, y
			}
		}
	}
	return 0, 0
}
