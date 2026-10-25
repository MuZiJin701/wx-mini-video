package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/internal/tui/ui"
)

func TestCategoryFilteringCountsAndOrder(t *testing.T) {
	m := mediaTestModel(t)
	m.candidates = []miniprogram.Candidate{
		{ID: "a", URL: "https://cdn.example.com/a.jpg", Kind: "image", AppID: "wx123"},
		{ID: "b", URL: "https://cdn.example.com/b.mp4", Kind: "video", AppID: "wx123"},
		{ID: "c", URL: "https://cdn.example.com/c.m3u8", Kind: "m3u8", AppID: "wx123"},
		{ID: "e", URL: "https://cdn.example.com/e.flv", Kind: "video", Live: true, AppID: "wx123"},
		{ID: "f", URL: "https://cdn.example.com/f.bin", Kind: "file", AppID: "wx123"},
	}
	m.otherCandidates = []miniprogram.Candidate{
		{ID: "d", URL: "https://cdn.example.com/d.mp3", Kind: "audio", SourceAppID: "wx999", Attribution: miniprogram.AttributionUnattributed},
	}
	if got := m.categoryCount(categoryAll); got != 6 {
		t.Fatalf("all count = %d", got)
	}
	if got := m.categoryCount(categoryImage); got != 1 {
		t.Fatalf("image count = %d", got)
	}
	if got := m.categoryCount(categoryAudio); got != 1 {
		t.Fatalf("audio count = %d", got)
	}
	if got := m.categoryCount(categoryLive); got != 1 {
		t.Fatalf("live count = %d", got)
	}
	if got := m.categoryCount(categorySegments); got != 1 {
		t.Fatalf("segments count = %d", got)
	}
	if got := m.categoryCount(categoryVideo); got != 1 {
		t.Fatalf("live candidate must not count as video, got video count %d", got)
	}
	m.setCategory(categoryVideo)
	if visible := m.filteredCandidates(); len(visible) != 1 || visible[0].ID != "b" {
		t.Fatalf("video filter = %#v", visible)
	}
	m.setCategory(categoryAll)
	m.nextCategory(1)
	if m.activeCategory() != categoryImage {
		t.Fatalf("tab cycle landed on %q", m.activeCategory())
	}
	view := m.View()
	for _, want := range []string{"1 全部(6)", "5 直播(1)", "6 分段媒体(1)", "?"} {
		if !strings.Contains(view, want) {
			t.Fatalf("tabs or attribution marker missing %q from view:\n%s", want, view)
		}
	}
}

func TestDefaultCategoryIsAll(t *testing.T) {
	if got := (model{}).activeCategory(); got != categoryAll {
		t.Fatalf("default category = %q, want all", got)
	}
	if len(categories) != 6 || categories[0] != categoryAll || categories[4] != categoryLive || categories[5] != categorySegments {
		t.Fatalf("category order = %#v", categories)
	}
}

func TestCtrlDNoLongerSwitchesTargetMode(t *testing.T) {
	m := mediaTestModel(t)
	m.candidates = testCandidates(2)
	m, cmd := targetKey(m, tea.KeyCtrlD, "")
	if cmd != nil || m.targetExpanded {
		t.Fatalf("Ctrl+D must not open the target editor anymore: cmd=%v expanded=%v", cmd, m.targetExpanded)
	}
	if !strings.Contains(m.View(), "1 全部") {
		t.Fatal("media view should stay visible after Ctrl+D")
	}
}

func TestDownloadedMarkerAndLocalPathInDrawer(t *testing.T) {
	m := mediaTestModel(t)
	m.candidates = testCandidates(2)
	m.downloaded[m.candidates[0].ID] = "downloads/wx123/视频/视频_20261025-130000_aaaaaaaa.mp4"
	m.showDetails = true
	m.selected = 0
	view := m.View()
	if !strings.Contains(view, "已下载") {
		t.Fatal("downloaded marker missing")
	}
	if !strings.Contains(view, "本地路径") {
		t.Fatal("detail drawer should show the local path")
	}
	if !strings.Contains(view, "本地缓存") {
		t.Fatal("detail drawer should keep cache info")
	}
	if !strings.Contains(view, "关闭") {
		t.Fatal("detail drawer should offer a close action")
	}
}

func TestAppearanceSwitchPersistsToUIJSON(t *testing.T) {
	m := testModel(t)
	if m.appearance != ui.AppearanceSystem {
		t.Fatalf("default appearance = %q", m.appearance)
	}
	cmd := m.setAppearance(ui.AppearanceDark)
	if !m.dark || cmd == nil {
		t.Fatal("switching to dark did not apply immediately")
	}
	msg := cmd()
	if saved, ok := msg.(appearanceSavedMsg); !ok || saved.err != nil {
		t.Fatalf("saving appearance failed: %#v", msg)
	}
	if got := ui.LoadAppearance(m.runtime.Settings.DownloadDir); got != ui.AppearanceDark {
		t.Fatalf("persisted appearance = %q", got)
	}
	cmd = m.cycleAppearance(-1)
	if m.appearance != ui.AppearanceLight {
		t.Fatalf("left should cycle back to light, got %q", m.appearance)
	}
	if cmd == nil {
		t.Fatal("appearance change should schedule a save")
	}
}

func TestAppearanceFocusDoesNotStealInputArrows(t *testing.T) {
	m := testModel(t)
	m, _ = typeText(m, "wx12")
	m, _ = targetKey(m, tea.KeyLeft, "")
	if m.inputCursor != 3 || m.appearance != ui.AppearanceSystem {
		t.Fatalf("left arrow in input should move the cursor, cursor=%d appearance=%q", m.inputCursor, m.appearance)
	}
	m, _ = targetKey(m, tea.KeyShiftTab, "")
	if m.focus != focusAppearance {
		t.Fatalf("shift+tab should reach the appearance control, focus=%v", m.focus)
	}
	m, _ = targetKey(m, tea.KeyRight, "")
	if m.appearance != ui.AppearanceLight {
		t.Fatalf("right arrow on appearance focus should change appearance, got %q", m.appearance)
	}
}

func TestAppearanceDropdownKeyboardAndMouse(t *testing.T) {
	m := mediaTestModel(t)
	m.width, m.height = 110, 40
	_ = m.View()
	control, ok := findHit(m, hitAppearanceControl)
	if !ok {
		t.Fatal("appearance control hit region missing")
	}
	m = clickHitRegion(t, m, control)
	if !m.appearanceOpen {
		t.Fatal("clicking the appearance control should open the dropdown")
	}
	_ = m.View()
	darkOption, ok := findHitIndex(m, hitAppearanceOption, appearanceIndex(ui.AppearanceDark))
	if !ok {
		t.Fatal("appearance option hit region missing")
	}
	m = clickHitRegion(t, m, darkOption)
	if m.appearance != ui.AppearanceDark || m.appearanceOpen {
		t.Fatalf("choosing dark should apply and close: appearance=%q open=%v", m.appearance, m.appearanceOpen)
	}

	// 键盘：聚焦外观后 Enter 打开，方向键移动选项，Enter 应用。
	keyboard := testModel(t)
	keyboard.appearance = ui.AppearanceSystem
	keyboard.focus = focusAppearance
	keyboard.appearanceOpen = false
	keyboard, _ = targetKey(keyboard, tea.KeyEnter, "")
	if !keyboard.appearanceOpen {
		t.Fatal("Enter on focused appearance should open the dropdown")
	}
	keyboard, _ = targetKey(keyboard, tea.KeyDown, "")
	keyboard, _ = targetKey(keyboard, tea.KeyEnter, "")
	if keyboard.appearance == ui.AppearanceSystem || keyboard.appearanceOpen {
		t.Fatalf("keyboard selection failed: appearance=%q open=%v", keyboard.appearance, keyboard.appearanceOpen)
	}
}

func TestMouseClicksTabs(t *testing.T) {
	m := mediaTestModel(t)
	m.candidates = testCandidates(2)
	m.width, m.height = 110, 40
	_ = m.View()
	tabHit, ok := findHitIndex(m, hitTab, 2)
	if !ok {
		t.Fatal("category tab hit region missing")
	}
	m = clickHitRegion(t, m, tabHit)
	if m.activeCategory() != categoryVideo {
		t.Fatalf("clicking third tab should select video, got %q", m.activeCategory())
	}
}

func TestViewLinesFitWidth(t *testing.T) {
	m := mediaTestModel(t)
	m.width, m.height = 110, 40
	m.candidates = testCandidates(3)
	for _, line := range strings.Split(m.View(), "\n") {
		if ui.Width(line) != m.width {
			t.Fatalf("line width = %d, want %d: %q", ui.Width(line), m.width, line)
		}
	}
}

func TestProgressAndNoticeStatus(t *testing.T) {
	m := mediaTestModel(t)
	m.downloading = true
	m.progress = minidownload.Progress{Status: "downloading", Downloaded: 1200, Total: 4800}
	view := m.View()
	if !strings.Contains(view, "下载中 25%") {
		t.Fatalf("progress percentage missing: %q", view)
	}
	m.downloading = false
	m.progress = minidownload.Progress{}
	m.setNotice("已保存 → downloads/wx123/图片/图片_20261025-130000_aaaaaaaa.jpg", time.Minute)
	_ = m.View()
	if !strings.Contains(m.notice, "已保存") {
		t.Fatal("notice not set")
	}
}

func TestNoNameReferencesAnywhere(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 110, 40
	if strings.Contains(m.View(), "名称") {
		t.Fatal("name feature still referenced in target view")
	}
	m.targetReady = true
	m.targetExpanded = false
	m.candidates = testCandidates(2)
	m.showDetails = true
	if strings.Contains(m.View(), "名称") {
		t.Fatal("name feature still referenced in media view")
	}
}

func TestHoverStateTracksRegions(t *testing.T) {
	m := mediaTestModel(t)
	m.candidates = testCandidates(1)
	m.width, m.height = 110, 40
	_ = m.View()
	tabHit, ok := findHit(m, hitTab)
	if !ok {
		t.Fatal("tab hit missing")
	}
	next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: tabHit.x + 1, Y: tabHit.y})
	hovered := next.(model)
	if !hovered.hits.isHovered(hitTab, tabHit.index) {
		t.Fatal("motion over a tab did not set hover state")
	}
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: 0, Y: 0})
	if next.(model).hits.hoverAction != hitNone {
		t.Fatal("hover state not cleared outside regions")
	}
}

func TestCandidateKindDisplayMarksLiveAndSegments(t *testing.T) {
	if got := candidateKindDisplay(miniprogram.Candidate{Kind: "video", Live: true}); got != "直播流" {
		t.Fatalf("live kind display = %q, want 直播流", got)
	}
	if got := candidateKindDisplay(miniprogram.Candidate{Kind: "video"}); got != "视频" {
		t.Fatalf("plain video kind display = %q, want 视频", got)
	}
	if got := candidateKindDisplay(miniprogram.Candidate{Kind: "m3u8", URL: "https://cdn.example.com/a.m3u8"}); got != "分段媒体" {
		t.Fatalf("m3u8 kind display = %q, want 分段媒体", got)
	}
	if !strings.Contains(candidateBasis(miniprogram.Candidate{Kind: "video", Live: true}), "直播流") {
		t.Fatal("live candidate basis should explain live detection")
	}
	if strings.Contains(candidateBasis(miniprogram.Candidate{Kind: "video", Live: true, Source: "json"}), "响应识别") {
		t.Fatal("suspected live candidate should keep a suspected marker")
	}
	if !strings.Contains(candidateBasis(miniprogram.Candidate{Kind: "m3u8"}), "m3u8/HLS") {
		t.Fatal("segment candidate basis should keep the m3u8/HLS format info")
	}
}

func TestProgressLineLiveStatuses(t *testing.T) {
	recording := progressLine(minidownload.Progress{Status: "recording", Downloaded: 8 * 1024 * 1024, Live: true})
	if !strings.Contains(recording, "录制直播中") || !strings.Contains(recording, "按 x 停止") {
		t.Fatalf("recording line = %q, want live recording hint", recording)
	}
	stopped := progressLine(minidownload.Progress{Status: "stopped", Live: true, Path: "downloads/直播/x.flv"})
	if !strings.Contains(stopped, "已保存") {
		t.Fatalf("stopped live line = %q, want saved path", stopped)
	}
	plain := progressLine(minidownload.Progress{Status: "stopped"})
	if !strings.Contains(plain, "下载已停止") {
		t.Fatalf("stopped download line = %q, want stop notice", plain)
	}
}

func TestStartLiveDownloadShowsRecordingHint(t *testing.T) {
	m := mediaTestModel(t)
	candidate := miniprogram.Candidate{
		ID:          "live-download",
		URL:         "https://play-t-live.example.com/7559_esd.flv",
		Kind:        "video",
		Suffix:      ".flv",
		Live:        true,
		Attribution: miniprogram.AttributionConfirmed,
	}
	next, cmd := m.startDownload(candidate)
	m = next.(model)
	if cmd == nil || !m.downloading || !m.downloadLive || m.downloadCancel == nil {
		t.Fatal("starting a live download should mark the model as recording")
	}
	m.downloadCancel()
	if logs := strings.Join(m.logs, "\n"); !strings.Contains(logs, "直播流") {
		t.Fatalf("logs = %q, want live stream hint", logs)
	}
	if !strings.Contains(m.notice, "录制") {
		t.Fatalf("notice = %q, want recording notice", m.notice)
	}
	m.candidates = []miniprogram.Candidate{candidate}
	m.category = categoryLive
	m.selected = 0
	m.selectedID = candidate.ID
	m.progress = minidownload.Progress{ID: candidate.ID, Status: "recording", Downloaded: 2 * 1024 * 1024, Live: true}
	view := m.View()
	for _, want := range []string{"直播流", "录制中", "录制直播中"} {
		if !strings.Contains(view, want) {
			t.Fatalf("media view missing %q for live recording:\n%s", want, view)
		}
	}
}

func TestProgressLineInterruptedStatus(t *testing.T) {
	line := progressLine(minidownload.Progress{
		Status: "interrupted",
		Live:   true,
		Path:   "downloads/wx123/直播/直播_1.flv",
		Error:  "直播流意外中断：unexpected EOF",
	})
	if !strings.Contains(line, "直播中断并保存") || !strings.Contains(line, "unexpected EOF") {
		t.Fatalf("interrupted line = %q, want saved path and reason", line)
	}
}

func TestDownloadInterruptedMarksRecordingSaved(t *testing.T) {
	m := mediaTestModel(t)
	next, _ := m.Update(downloadMsg{
		candidateID: "live-interrupted",
		path:        "downloads/wx123/直播/直播_1.flv",
		err:         fmt.Errorf("%w：unexpected EOF", minidownload.ErrRecordingInterrupted),
	})
	m = next.(model)
	if m.downloaded["live-interrupted"] == "" {
		t.Fatal("interrupted recording should be marked downloaded")
	}
	if !strings.Contains(strings.Join(m.logs, "\n"), "直播中断") {
		t.Fatalf("logs = %v, want interruption log", m.logs)
	}
	if !strings.Contains(m.notice, "已保存已录内容") {
		t.Fatalf("notice = %q, want saved recording notice", m.notice)
	}
}

func TestSelectionIdentityAcrossLiveCorrection(t *testing.T) {
	m := mediaTestModel(t)
	m.category = categoryLive
	live := miniprogram.Candidate{ID: "b", URL: "https://cdn.example.com/b.flv", Kind: "video", Live: true}
	m.candidates = []miniprogram.Candidate{live}
	m.selected = 0
	m.selectedID = live.ID
	// 实际响应确认是有限 FLV：候选应回到视频分类，选中不能落到别的资源上。
	m.candidates = []miniprogram.Candidate{{ID: live.ID, URL: live.URL, Kind: "video", Live: false}}
	m.restoreSelection(live.ID)
	if m.selected != -1 || m.selectedID != "" {
		t.Fatalf("removed selection should be cleared: selected=%d id=%q", m.selected, m.selectedID)
	}
	if !strings.Contains(m.notice, "移出当前分类") {
		t.Fatalf("notice = %q, want moved-category notice", m.notice)
	}
	_, cmd := m.downloadSelected()
	if cmd != nil || m.downloading {
		t.Fatal("download must not act on a different candidate after the selection was removed")
	}
}

func TestMouseQuitConfirmStartsStop(t *testing.T) {
	m := mediaTestModel(t)
	m.confirmQuit = true
	_ = m.View()
	region, ok := findHit(m, hitQuitConfirm)
	if !ok {
		t.Fatal("quit confirmation button hit missing")
	}
	hovered, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: region.x + 1, Y: region.y + 1})
	if h := hovered.(model); !h.hits.isHovered(hitQuitConfirm, 0) {
		t.Fatal("hover over quit confirmation button was ignored")
	}
	next, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: region.x + 1, Y: region.y + 1})
	m = next.(model)
	if cmd == nil || !m.stopping || m.confirmQuit {
		t.Fatalf("mouse quit confirmation should start the stop flow: cmd=%v stopping=%v confirmQuit=%v", cmd, m.stopping, m.confirmQuit)
	}
}

func TestMouseQuitConfirmDuringDownloadStopsFirst(t *testing.T) {
	m := mediaTestModel(t)
	m.downloading = true
	m.confirmQuit = true
	canceled := false
	m.downloadCancel = func() { canceled = true }
	_ = m.View()
	region, ok := findHit(m, hitQuitConfirm)
	if !ok {
		t.Fatal("quit confirmation button hit missing")
	}
	next, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: region.x + 1, Y: region.y + 1})
	m = next.(model)
	if cmd != nil || m.stopping || !m.quitAfterDownload || !canceled {
		t.Fatalf("quit during download should stop the download first: cmd=%v stopping=%v quitAfterDownload=%v canceled=%v", cmd, m.stopping, m.quitAfterDownload, canceled)
	}
}

func TestMouseLegacyKeepDismissesPrompt(t *testing.T) {
	m := testModel(t)
	m.legacyPrompt = true
	_ = m.View()
	region, ok := findHit(m, hitLegacyKeep)
	if !ok {
		t.Fatal("legacy certificate keep button hit missing")
	}
	next, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: region.x + 1, Y: region.y + 1})
	m = next.(model)
	if cmd == nil || m.legacyPrompt || !m.legacyChecked || !m.starting {
		t.Fatalf("mouse legacy keep should dismiss the prompt: cmd=%v prompt=%v checked=%v starting=%v", cmd, m.legacyPrompt, m.legacyChecked, m.starting)
	}
}

func TestMediaActionButtonsAndDisabledStates(t *testing.T) {
	m := mediaTestModel(t)
	m.candidates = testCandidates(2)
	m.width, m.height = 110, 40
	_ = m.View()
	download, ok := findHit(m, hitActionDownload)
	if !ok {
		t.Fatal("download button hit region missing")
	}
	next, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: download.x + 1, Y: download.y})
	m = next.(model)
	if cmd == nil || !m.downloading {
		t.Fatalf("clicking download should start a download: cmd=%v downloading=%v", cmd, m.downloading)
	}
	m.downloadCancel()

	// 下载中：再次下载与更换目标禁用，停止可用。
	m.downloading = true
	_ = m.View()
	if _, ok := findHit(m, hitActionDownload); ok {
		t.Fatal("download button should be disabled while downloading")
	}
	if _, ok := findHit(m, hitActionTargetSwitch); ok {
		t.Fatal("target switch button should be disabled while downloading")
	}
	stop, ok := findHit(m, hitActionStop)
	if !ok {
		t.Fatal("stop button should stay enabled while downloading")
	}
	canceled := false
	m.downloadCancel = func() { canceled = true }
	m = clickHitRegion(t, m, stop)
	if !canceled {
		t.Fatal("clicking stop should cancel the download context")
	}

	// 空列表：下载与详情不可用。
	empty := mediaTestModel(t)
	empty.width, empty.height = 110, 40
	_ = empty.View()
	if _, ok := findHit(empty, hitActionDownload); ok {
		t.Fatal("download should be disabled for an empty list")
	}
	if _, ok := findHit(empty, hitActionDetails); ok {
		t.Fatal("details should be disabled for an empty list")
	}
}

func TestDetailsAndLogsCloseButtons(t *testing.T) {
	media := mediaTestModel(t)
	media.candidates = testCandidates(2)
	media.showDetails = true
	media.width, media.height = 110, 40
	_ = media.View()
	drawerClose, ok := findHit(media, hitDrawerClose)
	if !ok {
		t.Fatal("details close hit region missing")
	}
	media = clickHitRegion(t, media, drawerClose)
	if media.showDetails {
		t.Fatal("clicking the close hint should hide the details drawer")
	}

	logs := mediaTestModel(t)
	logs.logs = []string{"line"}
	logs.showLogs = true
	logs.width, logs.height = 110, 40
	_ = logs.View()
	logClose, ok := findHit(logs, hitLogClose)
	if !ok {
		t.Fatal("log close hit region missing")
	}
	logs = clickHitRegion(t, logs, logClose)
	if logs.showLogs {
		t.Fatal("clicking the footer close action should hide the log overlay")
	}
}

func TestTargetAreaMouseConfirmInputAndCancel(t *testing.T) {
	m := testModel(t)
	m.started = true
	m.width, m.height = 110, 40
	_ = m.View()
	input, ok := findHit(m, hitTargetInput)
	if !ok {
		t.Fatal("target input hit region missing")
	}
	m.focus = focusNone
	m = clickHitRegion(t, m, input)
	if m.focus != focusInput {
		t.Fatalf("clicking the input should focus it, focus=%v", m.focus)
	}
	confirm, ok := findHit(m, hitTargetConfirm)
	if !ok {
		t.Fatal("confirm button hit region missing")
	}
	m.appIDInput = "wx123"
	m.inputCursor = len([]rune("wx123"))
	m = clickHitRegion(t, m, confirm)
	if !m.targetReady || m.targetExpanded {
		t.Fatal("clicking confirm should confirm and collapse the target area")
	}

	m.targetExpanded = true
	_ = m.View()
	cancel, ok := findHit(m, hitTargetCancel)
	if !ok {
		t.Fatal("cancel button hit region missing while switching")
	}
	m = clickHitRegion(t, m, cancel)
	if m.targetExpanded || !m.targetReady {
		t.Fatal("clicking cancel should collapse the editor and keep the target")
	}
}

func TestMouseProgramRowConfirmsTarget(t *testing.T) {
	m := testModel(t)
	m.started = true
	now := time.Now()
	m.programs = []miniprogram.Program{
		{AppID: "wx111", LastSeen: now},
		{AppID: "wx222", LastSeen: now},
	}
	m.width, m.height = 110, 40
	_ = m.View()
	row, ok := findHitIndex(m, hitTargetProgram, 1)
	if !ok {
		t.Fatal("program row hit region missing")
	}
	m = clickHitRegion(t, m, row)
	if !m.targetReady || m.runtime.Settings.Target.AppID != "wx222" {
		t.Fatalf("clicking a program should confirm it, target=%q", m.runtime.Settings.Target.AppID)
	}
}

func TestStartupRetryButtonAndStopErrorButtons(t *testing.T) {
	m := testModel(t)
	m.startErr = errors.New("certificate failed")
	m.width, m.height = 110, 40
	_ = m.View()
	retry, ok := findHit(m, hitTargetRetry)
	if !ok {
		t.Fatal("startup retry button hit region missing")
	}
	m = clickHitRegion(t, m, retry)
	if !m.starting {
		t.Fatal("clicking retry should start the proxy again")
	}

	failed := testModel(t)
	failed.stopErr = errors.New("restore failed")
	failed.width, failed.height = 110, 40
	_ = failed.View()
	stopRetry, ok := findHit(failed, hitStopRetry)
	if !ok {
		t.Fatal("stop error retry button hit region missing")
	}
	failed = clickHitRegion(t, failed, stopRetry)
	if !failed.stopping {
		t.Fatal("clicking retry on the stop error screen should retry cleanup")
	}

	forceModel := testModel(t)
	forceModel.stopErr = errors.New("restore failed")
	forceModel.width, forceModel.height = 110, 40
	_ = forceModel.View()
	force, ok := findHit(forceModel, hitStopForce)
	if !ok {
		t.Fatal("stop error force-quit button hit region missing")
	}
	next, cmd := forceModel.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: force.x + 1, Y: force.y})
	_ = next.(model)
	if cmd == nil {
		t.Fatal("clicking force quit should quit")
	}
}

func TestQuitCancelledWhenDownloadSaveFails(t *testing.T) {
	m := mediaTestModel(t)
	m.downloading = true
	m.quitAfterDownload = true
	next, _ := m.Update(downloadMsg{
		candidateID: "live-save-fail",
		path:        "downloads/wx/直播/直播.flv",
		err:         errors.New("已停止录制，但保存已录内容失败: rename 失败"),
	})
	m = next.(model)
	if m.stopping || m.quitAfterDownload {
		t.Fatalf("save failure should cancel the pending quit: stopping=%v quitAfterDownload=%v", m.stopping, m.quitAfterDownload)
	}
	if !strings.Contains(m.notice, "已取消退出") || !strings.Contains(strings.Join(m.logs, "\n"), "已取消退出") {
		t.Fatalf("save failure should keep the error visible: notice=%q logs=%v", m.notice, m.logs)
	}
}

func findHit(m model, action hitAction) (hitRegion, bool) {
	return findHitIndex(m, action, 0)
}

func findHitIndex(m model, action hitAction, index int) (hitRegion, bool) {
	for _, region := range m.hits.regions {
		if region.action == action && region.index == index {
			return region, true
		}
	}
	return hitRegion{}, false
}

func clickHit(t *testing.T, m model, region hitRegion) model {
	t.Helper()
	x := region.x + region.w/2
	y := region.y + region.h/2
	next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y})
	return next.(model)
}

// clickHitRegion 与 clickHit 相同，仅为新测试提供更直观的命名。
func clickHitRegion(t *testing.T, m model, region hitRegion) model {
	t.Helper()
	return clickHit(t, m, region)
}
