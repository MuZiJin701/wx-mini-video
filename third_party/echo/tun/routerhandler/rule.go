package routerhandler

import (
	"path/filepath"
)

// ProcessNameMatcher matches if the ConnectionOwner's process basename is in the given list.
type ProcessNameMatcher struct {
	names   []string
	nameMap map[string]bool
}

func NewProcessNameMatcher(names []string) *ProcessNameMatcher {
	m := &ProcessNameMatcher{
		names:   names,
		nameMap: make(map[string]bool),
	}
	for _, name := range names {
		m.nameMap[name] = true
	}
	return m
}

func (m *ProcessNameMatcher) Match(owner *ConnectionOwner) bool {
	if owner == nil || owner.ProcessPath == "" {
		return false
	}
	return m.nameMap[filepath.Base(owner.ProcessPath)]
}

// ProcessPathMatcher matches if the ConnectionOwner's full process path is in the given list.
type ProcessPathMatcher struct {
	paths   []string
	pathMap map[string]bool
}

func NewProcessPathMatcher(paths []string) *ProcessPathMatcher {
	m := &ProcessPathMatcher{
		paths:   paths,
		pathMap: make(map[string]bool),
	}
	for _, p := range paths {
		m.pathMap[p] = true
	}
	return m
}

func (m *ProcessPathMatcher) Match(owner *ConnectionOwner) bool {
	if owner == nil {
		return false
	}
	if owner.ProcessPath != "" && m.pathMap[owner.ProcessPath] {
		return true
	}
	for _, pkg := range owner.AndroidPackageNames {
		if m.pathMap[pkg] {
			return true
		}
	}
	return false
}

// UserMatcher matches if the ConnectionOwner's user name is in the given list.
type UserMatcher struct {
	users   []string
	userMap map[string]bool
}

func NewUserMatcher(users []string) *UserMatcher {
	m := &UserMatcher{
		users:   users,
		userMap: make(map[string]bool),
	}
	for _, u := range users {
		m.userMap[u] = true
	}
	return m
}

func (m *UserMatcher) Match(owner *ConnectionOwner) bool {
	if owner == nil || owner.UserName == "" {
		return false
	}
	return m.userMap[owner.UserName]
}

// UserIdMatcher matches if the ConnectionOwner's user ID is in the given list.
type UserIdMatcher struct {
	ids   []int32
	idMap map[int32]bool
}

func NewUserIdMatcher(ids []int32) *UserIdMatcher {
	m := &UserIdMatcher{
		ids:   ids,
		idMap: make(map[int32]bool),
	}
	for _, id := range ids {
		m.idMap[id] = true
	}
	return m
}

func (m *UserIdMatcher) Match(owner *ConnectionOwner) bool {
	if owner == nil || owner.UserId == -1 {
		return false
	}
	return m.idMap[owner.UserId]
}
