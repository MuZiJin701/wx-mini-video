//go:build windows

package windows

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Action represents what to do with matched traffic.
type Action int

const (
	// ActionProxy routes traffic through the Echo proxy.
	ActionProxy Action = iota
	// ActionDirect allows traffic to pass through unmodified.
	ActionDirect
	// ActionBlock drops the traffic silently.
	ActionBlock
)

func (a Action) String() string {
	switch a {
	case ActionProxy:
		return "PROXY"
	case ActionDirect:
		return "DIRECT"
	case ActionBlock:
		return "BLOCK"
	default:
		return fmt.Sprintf("Action(%d)", int(a))
	}
}

// ProcessRule defines a rule for matching process traffic.
type ProcessRule struct {
	ID          uint32
	ProcessName string // "chrome.exe", "fire*", "*"
	TargetHosts string // "*", "192.168.*.*", "10.0.0.1;172.16.0.0"
	TargetPorts string // "*", "80;443", "8000-9000"
	Action      Action
	Enabled     bool
}

// RuleManager manages an ordered list of process rules.
type RuleManager struct {
	mu       sync.RWMutex
	rules    []*ProcessRule
	nextID   uint32
	hasRules atomic.Bool
}

// NewRuleManager creates a new rule manager.
func NewRuleManager() *RuleManager {
	return &RuleManager{}
}

// AddRule adds a rule and returns its assigned ID.
func (rm *RuleManager) AddRule(rule *ProcessRule) uint32 {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.nextID++
	rule.ID = rm.nextID
	rm.rules = append(rm.rules, rule)
	rm.updateHasActiveRules()
	return rule.ID
}

// RemoveRule removes a rule by ID.
func (rm *RuleManager) RemoveRule(id uint32) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for i, r := range rm.rules {
		if r.ID == id {
			rm.rules = append(rm.rules[:i], rm.rules[i+1:]...)
			rm.updateHasActiveRules()
			return nil
		}
	}
	return fmt.Errorf("rule %d not found", id)
}

// EnableRule enables a rule by ID.
func (rm *RuleManager) EnableRule(id uint32) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, r := range rm.rules {
		if r.ID == id {
			r.Enabled = true
			rm.updateHasActiveRules()
			return nil
		}
	}
	return fmt.Errorf("rule %d not found", id)
}

// DisableRule disables a rule by ID.
func (rm *RuleManager) DisableRule(id uint32) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, r := range rm.rules {
		if r.ID == id {
			r.Enabled = false
			rm.updateHasActiveRules()
			return nil
		}
	}
	return fmt.Errorf("rule %d not found", id)
}

func (rm *RuleManager) updateHasActiveRules() {
	for _, r := range rm.rules {
		if r.Enabled {
			rm.hasRules.Store(true)
			return
		}
	}
	rm.hasRules.Store(false)
}

// HasActiveRules returns true if there is at least one enabled rule.
func (rm *RuleManager) HasActiveRules() bool {
	return rm.hasRules.Load()
}

// Match evaluates rules in order and returns the action for the given process/dest.
// processPath is the full path of the process executable.
// destIP is in network byte order, destPort is in host byte order.
func (rm *RuleManager) Match(processPath string, destIP uint32, destPort uint16) Action {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var wildcardRule *ProcessRule

	for _, r := range rm.rules {
		if !r.Enabled {
			continue
		}

		isWildcardProcess := r.ProcessName == "*" || r.ProcessName == "" || strings.EqualFold(r.ProcessName, "ANY")

		if isWildcardProcess {
			// Check if it has specific IP/port filters
			hasFilters := (r.TargetHosts != "*" && r.TargetHosts != "") ||
				(r.TargetPorts != "*" && r.TargetPorts != "")

			if hasFilters {
				// Filtered wildcard: check IP/port now
				if matchIPList(r.TargetHosts, destIP) && matchPortList(r.TargetPorts, destPort) {
					return r.Action
				}
				continue
			}

			// Unfiltered wildcard: save as fallback
			if wildcardRule == nil {
				wildcardRule = r
			}
			continue
		}

		// Specific process rule
		if matchProcessList(r.ProcessName, processPath) {
			if matchIPList(r.TargetHosts, destIP) && matchPortList(r.TargetPorts, destPort) {
				return r.Action
			}
		}
	}

	if wildcardRule != nil {
		return wildcardRule.Action
	}
	return ActionDirect
}

// --- Process name matching ---

func matchProcessList(pattern, processPath string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	for _, token := range splitTokens(pattern, ",;") {
		token = strings.TrimSpace(token)
		token = strings.Trim(token, "\"")
		if token == "" {
			continue
		}
		if matchProcessPattern(token, processPath) {
			return true
		}
	}
	return false
}

func matchProcessPattern(pattern, processFullPath string) bool {
	if pattern == "*" {
		return true
	}

	// Decide whether to match against full path or just filename
	target := processFullPath
	if !strings.ContainsAny(pattern, "\\/") {
		target = filepath.Base(processFullPath)
	}

	// Check for wildcard
	starIdx := strings.IndexByte(pattern, '*')
	if starIdx < 0 {
		// Exact match (case insensitive)
		return strings.EqualFold(target, pattern)
	}

	prefix := pattern[:starIdx]
	suffix := pattern[starIdx+1:]

	if len(target) < len(prefix)+len(suffix) {
		return false
	}

	if prefix != "" && !strings.HasPrefix(strings.ToLower(target), strings.ToLower(prefix)) {
		return false
	}
	if suffix != "" && !strings.HasSuffix(strings.ToLower(target), strings.ToLower(suffix)) {
		return false
	}
	return true
}

// --- IP matching ---

func matchIPList(ipList string, ip uint32) bool {
	if ipList == "" || ipList == "*" {
		return true
	}
	for _, token := range splitTokens(ipList, ";,") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if matchIPPattern(token, ip) {
			return true
		}
	}
	return false
}

func matchIPPattern(pattern string, ip uint32) bool {
	if pattern == "*" {
		return true
	}

	// IP range: "10.0.0.1-10.0.0.255"
	if dashIdx := strings.IndexByte(pattern, '-'); dashIdx >= 0 {
		startStr := pattern[:dashIdx]
		endStr := pattern[dashIdx+1:]
		startIP := parseIPToUint32(startStr)
		endIP := parseIPToUint32(endStr)
		if startIP == 0 && endIP == 0 {
			return false
		}
		return ip >= startIP && ip <= endIP
	}

	// Octet matching with wildcards: "192.168.*.*"
	octets := strings.Split(pattern, ".")
	if len(octets) != 4 {
		return false
	}

	ipBytes := [4]byte{
		byte(ip >> 24),
		byte(ip >> 16),
		byte(ip >> 8),
		byte(ip),
	}

	for i := 0; i < 4; i++ {
		if octets[i] == "*" {
			continue
		}
		val, err := strconv.Atoi(octets[i])
		if err != nil {
			return false
		}
		if byte(val) != ipBytes[i] {
			return false
		}
	}
	return true
}

func parseIPToUint32(s string) uint32 {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return 0
	}
	var result uint32
	for i := 0; i < 4; i++ {
		v, err := strconv.Atoi(parts[i])
		if err != nil || v < 0 || v > 255 {
			return 0
		}
		result = (result << 8) | uint32(v)
	}
	return result
}

// --- Port matching ---

func matchPortList(portList string, port uint16) bool {
	if portList == "" || portList == "*" {
		return true
	}
	for _, token := range splitTokens(portList, ",;") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if matchPortPattern(token, port) {
			return true
		}
	}
	return false
}

func matchPortPattern(pattern string, port uint16) bool {
	if pattern == "*" {
		return true
	}

	// Port range: "8000-9000"
	if dashIdx := strings.IndexByte(pattern, '-'); dashIdx >= 0 {
		start, err1 := strconv.Atoi(pattern[:dashIdx])
		end, err2 := strconv.Atoi(pattern[dashIdx+1:])
		if err1 != nil || err2 != nil {
			return false
		}
		return int(port) >= start && int(port) <= end
	}

	val, err := strconv.Atoi(pattern)
	if err != nil {
		return false
	}
	return uint16(val) == port
}

// BuildPortFilter returns a WinDivert filter expression covering all target
// ports from enabled rules. Returns ("", false) if any enabled rule uses
// wildcard ports ("*" or empty), meaning the filter cannot be narrowed.
// Rules must be added before calling Start() for port-specific filtering.
func (rm *RuleManager) BuildPortFilter() (string, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	seen := make(map[string]bool)
	var clauses []string

	for _, r := range rm.rules {
		if !r.Enabled {
			continue
		}
		ports := r.TargetPorts
		if ports == "" || ports == "*" {
			return "", false
		}
		for _, token := range splitTokens(ports, ",;") {
			token = strings.TrimSpace(token)
			if token == "" || token == "*" {
				return "", false
			}
			if seen[token] {
				continue
			}
			seen[token] = true

			if dashIdx := strings.IndexByte(token, '-'); dashIdx >= 0 {
				start := token[:dashIdx]
				end := token[dashIdx+1:]
				clauses = append(clauses, fmt.Sprintf("(tcp.DstPort >= %s and tcp.DstPort <= %s)", start, end))
			} else {
				clauses = append(clauses, fmt.Sprintf("tcp.DstPort == %s", token))
			}
		}
	}

	if len(clauses) == 0 {
		return "", false
	}
	return strings.Join(clauses, " or "), true
}

// --- Utility ---

func splitTokens(s string, delims string) []string {
	f := func(r rune) bool {
		return strings.ContainsRune(delims, r)
	}
	return strings.FieldsFunc(s, f)
}

// IsBroadcastOrMulticast returns true if the IP (network byte order) is
// a broadcast, multicast, or APIPA address.
func IsBroadcastOrMulticast(ip uint32) bool {
	firstOctet := byte(ip >> 24)

	// Broadcast: 255.255.255.255
	if ip == 0xFFFFFFFF {
		return true
	}
	// Subnet broadcast: x.x.x.255
	if byte(ip) == 0xFF {
		return true
	}
	// Multicast: 224.0.0.0 - 239.255.255.255
	if firstOctet >= 224 && firstOctet <= 239 {
		return true
	}
	// APIPA: 169.254.x.x
	if firstOctet == 169 && byte(ip>>16) == 254 {
		return true
	}
	return false
}
