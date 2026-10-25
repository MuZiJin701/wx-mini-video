package miniprogram

import (
	"sort"
	"strings"
	"time"
)

// Program 记录本次运行中观察到的小程序，只保留发现列表需要的元数据。
type Program struct {
	AppID     string    `json:"app_id"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// now 是测试注入点：发现列表的首次/最近出现时间统一从这里获取。
var now = time.Now

// SetTarget 切换目标：清空目标候选并让在途旧响应失效，但按 ADR 0008 保留
// 与目标无关的未归属列表，避免用户切换目标后丢失 H5 视频。
func (s *Store) SetTarget(target Target) {
	target.AppID = strings.TrimSpace(target.AppID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.target = target
	s.generation++
	s.items = make(map[string]Candidate)
	s.order = nil
}

func (s *Store) IsCurrentTarget(generation uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.generation == generation && s.target.AppID != ""
}

func (s *Store) Observe(appID string) (Target, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observe(appID)
	return s.target, s.generation
}

// observe 更新发现列表：已有条目只刷新最近出现时间，新条目首次出现与最近出现时间相同。
func (s *Store) observe(appID string) {
	if appID == "" {
		return
	}
	observedAt := now()
	for i := range s.programs {
		if s.programs[i].AppID == appID {
			s.programs[i].LastSeen = observedAt
			return
		}
	}
	s.programs = append(s.programs, Program{AppID: appID, FirstSeen: observedAt, LastSeen: observedAt})
}

// Programs 返回发现列表的副本：最近出现的排在前面；最近出现相同的按首次出现升序。
func (s *Store) Programs() []Program {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]Program(nil), s.programs...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		return out[i].FirstSeen.Before(out[j].FirstSeen)
	})
	return out
}
