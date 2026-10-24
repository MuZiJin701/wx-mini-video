package miniprogram

import "strings"

// SetTarget 切换目标：清空目标候选并让在途旧响应失效，但按 ADR 0008 保留
// 与目标无关的未归属列表，避免用户切换目标后丢失 H5 视频。
func (s *Store) SetTarget(target Target) {
	target.AppID = strings.TrimSpace(target.AppID)
	target.Name = strings.TrimSpace(target.Name)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.target = target
	s.generation++
	s.items = make(map[string]Candidate)
	s.order = nil
	if target.AppID != "" {
		for i := range s.programs {
			if s.programs[i].AppID == target.AppID {
				s.programs[i].Name = target.Name
			}
		}
	}
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

func (s *Store) observe(appID string) {
	if appID == "" {
		return
	}
	for _, program := range s.programs {
		if program.AppID == appID {
			return
		}
	}
	program := Target{AppID: appID}
	if appID == s.target.AppID {
		program.Name = s.target.Name
	}
	s.programs = append(s.programs, program)
}

func (s *Store) Programs() []Target {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Target(nil), s.programs...)
}
