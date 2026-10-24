package tun

import (
	"encoding/json"
	"fmt"
	"os"
)

// TunConfig is the top-level configuration for the TUN forwarder.
type TunConfig struct {
	Enabled   bool             `json:"enabled"`
	Inbound   InboundConfig    `json:"inbound"`
	Outbounds []OutboundConfig `json:"outbounds"`
	Route     RouteConfig      `json:"route"`
	DNS       DNSConfig        `json:"dns"`
}

// InboundConfig defines the TUN device settings.
type InboundConfig struct {
	TunName      string `json:"tun_name"`
	Inet4Address string `json:"inet4_address"`
	MTU          uint32 `json:"mtu"`
	AutoRoute    bool   `json:"auto_route"`
	StrictRoute  bool   `json:"strict_route"`
	Sniff        bool   `json:"sniff"`
}

// OutboundConfig defines an outbound (direct / http / socks5).
type OutboundConfig struct {
	Tag      string `json:"tag"`
	Type     string `json:"type"`
	Server   string `json:"server,omitempty"`
	Port     uint16 `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// RouteConfig holds routing rules and default outbound.
type RouteConfig struct {
	Rules            []RuleConfig `json:"rules"`
	Final            string       `json:"final"`
	DefaultInterface string       `json:"default_interface,omitempty"`
}

// RuleConfig defines a single routing rule.
type RuleConfig struct {
	ProcessName  []string `json:"process_name,omitempty"`
	DomainSuffix []string `json:"domain_suffix,omitempty"`
	Domain       []string `json:"domain,omitempty"`
	IPCidr       []string `json:"ip_cidr,omitempty"`
	Port         []uint16 `json:"port,omitempty"`
	Protocol     string   `json:"protocol,omitempty"`
	Invert       bool     `json:"invert,omitempty"`
	Outbound     string   `json:"outbound"`
}

// DNSConfig defines DNS behavior.
type DNSConfig struct {
	FakeDNS      bool   `json:"fake_dns"`
	FakeDNSRange string `json:"fake_dns_range"`
}

func LoadConfig(path string) (*TunConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg TunConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func DefaultConfig() *TunConfig {
	return &TunConfig{
		Enabled: true,
		Inbound: InboundConfig{
			TunName:      "",
			Inet4Address: "10.99.99.1/30",
			MTU:          1500,
			AutoRoute:    true,
			StrictRoute:  true,
			Sniff:        true,
		},
		Outbounds: []OutboundConfig{
			{Tag: "proxy", Type: "http", Server: "127.0.0.1", Port: 8899},
			{Tag: "direct", Type: "direct"},
		},
		Route: RouteConfig{
			Rules: []RuleConfig{},
			Final: "direct",
		},
		DNS: DNSConfig{
			FakeDNS:      true,
			FakeDNSRange: "198.18.0.0/15",
		},
	}
}
