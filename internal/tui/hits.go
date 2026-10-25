package tui

// hitAction 是鼠标命中区域的动作类型。
type hitAction int

const (
	hitNone hitAction = iota

	// 品牌区外观下拉
	hitAppearanceControl
	hitAppearanceOption // index 为外观选项下标

	// 目标选择区
	hitTargetInput
	hitTargetProgram // index 为发现列表下标
	hitTargetConfirm
	hitTargetCancel
	hitTargetRetry

	// 媒体视图
	hitTab // index 为分类下标
	hitRow // index 为可见候选下标

	hitActionDownload
	hitActionStop
	hitActionDetails
	hitActionLogs
	hitActionRefresh
	hitActionOpenDir
	hitActionClear
	hitActionTargetSwitch
	hitActionQuit
	hitGitHub

	hitDrawerClose
	hitLogClose

	// 模态覆盖层与恢复失败界面
	hitQuitConfirm
	hitOverlayCancel
	hitLegacyClean
	hitLegacyKeep
	hitStopRetry
	hitStopForce
)

type rect struct {
	x, y, w, h int
}

func (r rect) contains(x, y int) bool {
	return r.w > 0 && r.h > 0 && x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

type hitRegion struct {
	rect
	action hitAction
	index  int
}

// hitState 保存上一帧渲染出的鼠标命中区域与悬停状态。
type hitState struct {
	regions     []hitRegion
	hoverAction hitAction
	hoverIndex  int
	lastClickAt int64 // Unix 毫秒
	lastClickID string
}

func newHitState() *hitState {
	return &hitState{hoverAction: hitNone, hoverIndex: -1, lastClickAt: -1}
}

func (h *hitState) reset() {
	h.regions = h.regions[:0]
}

func (h *hitState) add(x, y, w, height int, action hitAction, index int) {
	if w <= 0 || height <= 0 {
		return
	}
	h.regions = append(h.regions, hitRegion{rect: rect{x: x, y: y, w: w, h: height}, action: action, index: index})
}

func (h *hitState) find(x, y int) (hitRegion, bool) {
	for i := len(h.regions) - 1; i >= 0; i-- {
		if h.regions[i].contains(x, y) {
			return h.regions[i], true
		}
	}
	return hitRegion{}, false
}

func (h *hitState) setHover(action hitAction, index int) bool {
	if h.hoverAction == action && h.hoverIndex == index {
		return false
	}
	h.hoverAction = action
	h.hoverIndex = index
	return true
}

func (h *hitState) clearHover() bool {
	if h.hoverAction == hitNone && h.hoverIndex == -1 {
		return false
	}
	h.hoverAction = hitNone
	h.hoverIndex = -1
	return true
}

func (h *hitState) isHovered(action hitAction, index int) bool {
	return h.hoverAction == action && h.hoverIndex == index
}
