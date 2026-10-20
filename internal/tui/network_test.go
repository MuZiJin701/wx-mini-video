package tui

import (
	"errors"
	"testing"
)

func TestNetworkSwitchStopsSniffingAndWaitsForCleanup(t *testing.T) {
	m := model{runtime: targetRuntime(t), started: true, networkGen: 2}
	updated, cmd := m.Update(networkMsg{generation: 2, err: errors.New("network changed")})
	state := updated.(model)
	if cmd == nil || state.started || !state.networkPause || state.startErr == nil {
		t.Fatal("network change did not pause the proxy")
	}
	updated, cmd = state.Update(networkStoppedMsg{})
	state = updated.(model)
	if cmd != nil || state.networkPause || state.started || state.stopErr != nil {
		t.Fatal("cleanup should leave a retryable paused UI")
	}
	updated, cmd = state.Update(networkMsg{generation: 2, err: errors.New("stale")})
	if cmd != nil || updated.(model).networkPause {
		t.Fatal("stale watcher stopped another generation")
	}
	failed := errors.New("restore denied")
	updated, _ = state.Update(networkStoppedMsg{err: failed})
	if !errors.Is(updated.(model).stopErr, failed) {
		t.Fatal("lost cleanup failure")
	}
}
