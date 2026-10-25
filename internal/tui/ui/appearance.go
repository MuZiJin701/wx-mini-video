package ui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// AppearanceMode 是外观模式的用户选择：跟随系统、浅色或深色。
type AppearanceMode string

const (
	AppearanceSystem AppearanceMode = "system"
	AppearanceLight  AppearanceMode = "light"
	AppearanceDark   AppearanceMode = "dark"
)

const AppearanceFileName = "ui.json"

type appearanceFile struct {
	Appearance AppearanceMode `json:"appearance"`
}

// Valid 报告外观模式是否为可识别的值。
func (m AppearanceMode) Valid() bool {
	switch m {
	case AppearanceSystem, AppearanceLight, AppearanceDark:
		return true
	default:
		return false
	}
}

// DisplayName 返回面向用户的外观模式名称。
func (m AppearanceMode) DisplayName() string {
	switch m {
	case AppearanceLight:
		return "浅色"
	case AppearanceDark:
		return "深色"
	default:
		return "跟随系统"
	}
}

// LoadAppearance 从下载目录下的 ui.json 读取外观偏好；缺失或非法时回退跟随系统。
func LoadAppearance(dir string) AppearanceMode {
	if dir == "" {
		return AppearanceSystem
	}
	data, err := os.ReadFile(filepath.Join(dir, AppearanceFileName))
	if err != nil {
		return AppearanceSystem
	}
	var file appearanceFile
	if err := json.Unmarshal(data, &file); err != nil || !file.Appearance.Valid() {
		return AppearanceSystem
	}
	return file.Appearance
}

// SaveAppearance 原子写入外观偏好，避免半写入文件。
func SaveAppearance(dir string, mode AppearanceMode) error {
	if dir == "" {
		return errors.New("下载目录为空")
	}
	if !mode.Valid() {
		return errors.New("无效的外观模式")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(appearanceFile{Appearance: mode})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(dir, ".ui-*.json")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, filepath.Join(dir, AppearanceFileName))
}

// NextAppearance 按 跟随系统 → 浅色 → 深色 → 跟随系统 循环。
func NextAppearance(mode AppearanceMode) AppearanceMode {
	switch mode {
	case AppearanceLight:
		return AppearanceDark
	case AppearanceDark:
		return AppearanceSystem
	default:
		return AppearanceLight
	}
}

// ResolveDark 把外观模式解析为实际的明暗状态；跟随系统时读取系统偏好。
func ResolveDark(mode AppearanceMode) bool {
	switch mode {
	case AppearanceLight:
		return false
	case AppearanceDark:
		return true
	default:
		return SystemDark()
	}
}
