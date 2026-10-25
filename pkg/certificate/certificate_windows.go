//go:build windows

package certificate

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const (
	installCertArgPrefix = "--wx-mini-video-install-cert="
	removeLegacyArg      = "--wx-mini-video-remove-legacy-root"
)

// processElevated 是测试注入点。主程序以当前用户运行，只有证书安装/旧证书清理需要提权。
var processElevated = func() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// RunHelper 在提权子进程中执行单个证书操作后退出；普通进程返回 handled=false。
// 证书通过命令行 base64 传递，避免提权后跨用户临时目录权限问题时找不到文件。
func RunHelper(args []string) (bool, error) {
	for _, arg := range args {
		if strings.HasPrefix(arg, installCertArgPrefix) {
			encoded := strings.TrimPrefix(arg, installCertArgPrefix)
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err == nil {
				err = installCertificateDirect(data)
			}
			return true, err
		}
		if arg == removeLegacyArg {
			return true, removeLegacyRootDirect(LegacyRootThumbprint)
		}
	}
	return false, nil
}

func fetchCertificates() ([]Certificate, error) {
	certificates, err := fetchCertificatesWithPowerShell()
	if err == nil {
		return certificates, nil
	}
	if fallback, fallbackErr := fetchCertificatesWithCertutil(); fallbackErr == nil {
		return fallback, nil
	}
	return nil, err
}

func fetchCertificatesWithPowerShell() ([]Certificate, error) {
	// PowerShell 2.0 compatible command
	cmd := "$ErrorActionPreference='Stop'; Get-ChildItem Cert:\\LocalMachine\\Root, Cert:\\CurrentUser\\Root | ForEach-Object { $_.Thumbprint + \"###\" + $_.Subject }"
	ps := exec.Command("powershell.exe", "-NoProfile", "-Command", cmd)
	output, err2 := ps.CombinedOutput()
	if err2 != nil {
		return nil, fmt.Errorf("获取证书时发生错误，%v, %s\n", err2.Error(), strings.TrimSpace(string(output)))
	}

	var certificates []Certificate
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "###", 2)
		if len(parts) < 2 {
			continue
		}
		thumbprint := parts[0]
		subject_str := parts[1]

		subj := parseCertificateSubject(subject_str)
		certificates = append(certificates, Certificate{
			Thumbprint: thumbprint,
			Subject:    subj,
		})
	}
	return certificates, nil
}

func fetchCertificatesWithCertutil() ([]Certificate, error) {
	var certificates []Certificate
	for _, args := range [][]string{{"-store", "Root"}, {"-user", "-store", "Root"}} {
		output, err := exec.Command("certutil", args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("certutil 获取证书失败: %w, %s", err, strings.TrimSpace(string(output)))
		}
		certificates = append(certificates, parseCertutilCertificates(string(output))...)
	}
	return certificates, nil
}

func parseCertificateSubject(subject string) CertificateSubject {
	subj := CertificateSubject{}
	pairs := strings.Split(subject, ",")
	for _, p := range pairs {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := kv[0]
		value := kv[1]
		switch key {
		case "CN":
			subj.CN = value
		case "OU":
			subj.OU = value
		case "O":
			subj.O = value
		case "L":
			subj.L = value
		case "S":
			subj.S = value
		case "C":
			subj.C = value
		}
	}
	return subj
}

func parseCertutilCertificates(output string) []Certificate {
	thumbprintPattern := regexp.MustCompile(`(?i)(?:Cert Hash\(sha1\)|证书哈希\(sha1\)):\s*([0-9a-f ]+)`)
	var certificates []Certificate
	var current *Certificate
	flush := func() {
		if current != nil && (current.Thumbprint != "" || current.Subject.CN != "") {
			certificates = append(certificates, *current)
		}
		current = nil
	}
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.Contains(line, "===============") {
			flush()
			current = &Certificate{}
			continue
		}
		if current == nil {
			current = &Certificate{}
		}
		if strings.HasPrefix(line, "Subject:") {
			current.Subject = parseCertificateSubject(strings.TrimSpace(strings.TrimPrefix(line, "Subject:")))
			continue
		}
		if strings.HasPrefix(line, "使用者:") {
			current.Subject = parseCertificateSubject(strings.TrimSpace(strings.TrimPrefix(line, "使用者:")))
			continue
		}
		if match := thumbprintPattern.FindStringSubmatch(line); len(match) == 2 {
			current.Thumbprint = strings.ToUpper(strings.ReplaceAll(match[1], " ", ""))
		}
	}
	flush()
	return certificates
}

func installCertificate(cert_data []byte) error {
	if processElevated() {
		return installCertificateDirect(cert_data)
	}
	thumbprint, err := Thumbprint(cert_data)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(cert_data)
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位程序失败: %w", err)
	}
	directory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("读取当前目录失败: %w", err)
	}
	if err := launchElevated(executable, []string{installCertArgPrefix + encoded}, directory); err != nil {
		return fmt.Errorf("请求管理员权限安装根证书失败: %w", err)
	}
	if err := waitForCertificate(thumbprint, true); err != nil {
		return fmt.Errorf("安装根证书失败: %w", err)
	}
	return nil
}

// installCertificateDirect 由提权子进程调用，假定当前进程已有管理员权限。
func installCertificateDirect(cert_data []byte) error {
	cert_file, err := os.CreateTemp("", "WxMiniVideoRoot-*.cer")
	if err != nil {
		return fmt.Errorf("没有创建证书的权限，%v\n", err.Error())
	}
	defer os.Remove(cert_file.Name())
	defer cert_file.Close()
	if _, err := cert_file.Write(cert_data); err != nil {
		return fmt.Errorf("获取证书失败，%v\n", err.Error())
	}
	if err := cert_file.Close(); err != nil {
		return fmt.Errorf("生成证书失败，%v\n", err.Error())
	}
	// Use certutil for Windows 7 compatibility
	cmd := exec.Command("certutil", "-addstore", "Root", cert_file.Name())
	output, err2 := cmd.CombinedOutput()
	if err2 != nil {
		return fmt.Errorf("安装证书时发生错误，%v\n", string(output))
	}
	return nil
}

var runCertificateDelete = func(args ...string) ([]byte, error) { return exec.Command("certutil", args...).CombinedOutput() }

// launchElevated 是测试注入点；生产环境通过 runas 启动本程序的提权子进程。
var launchElevated = launchElevatedHelper

func uninstallCertificate(thumbprint string) error {
	if thumbprint != LegacyRootThumbprint {
		return fmt.Errorf("只允许按固定指纹清理旧共享根证书")
	}
	if processElevated() {
		return removeLegacyRootDirect(thumbprint)
	}
	// 当前用户根证书库用普通权限即可清理；提权子进程运行在管理员账号下，
	// 无法删除标准用户自己的 CurrentUser\Root。
	_, _ = runCertificateDelete("-user", "-delstore", "Root", thumbprint)
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位程序失败: %w", err)
	}
	directory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("读取当前目录失败: %w", err)
	}
	if err := launchElevated(executable, []string{removeLegacyArg}, directory); err != nil {
		return fmt.Errorf("请求管理员权限清理旧共享根证书失败: %w", err)
	}
	if err := waitForCertificate(thumbprint, false); err != nil {
		return fmt.Errorf("清理旧共享根证书失败: %w", err)
	}
	return nil
}

// removeLegacyRootDirect 假定当前进程已有管理员权限。
func removeLegacyRootDirect(thumbprint string) error {
	var cleanupErr error
	for _, args := range [][]string{{"-delstore", "Root", thumbprint}, {"-user", "-delstore", "Root", thumbprint}} {
		output, err := runCertificateDelete(args...)
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("删除证书失败: %w, %s", err, strings.TrimSpace(string(output))))
		}
	}
	return cleanupErr
}

func waitForCertificate(thumbprint string, present bool) error {
	deadline := time.Now().Add(2 * time.Minute)
	var lastErr error
	for {
		matched, err := CheckHasCertificate(thumbprint)
		if err == nil {
			lastErr = nil
			if matched == present {
				return nil
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("等待管理员授权或检查证书超时: %w", lastErr)
			}
			return fmt.Errorf("等待管理员授权或证书操作完成超时，可能已取消 UAC")
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func launchElevatedHelper(executable string, args []string, directory string) error {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	parameters, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return err
	}
	cwd, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, verb, file, parameters, cwd, 0); err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return fmt.Errorf("已取消管理员授权")
		}
		return err
	}
	return nil
}
