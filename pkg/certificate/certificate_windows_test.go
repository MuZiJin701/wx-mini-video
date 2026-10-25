//go:build windows

package certificate

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestLegacyRootRemovalTargetsBothStoresAndVerifiesResult(t *testing.T) {
	originalDelete, originalFetch := runCertificateDelete, fetchCertificatesForCheck
	originalElevated := processElevated
	t.Cleanup(func() {
		runCertificateDelete, fetchCertificatesForCheck = originalDelete, originalFetch
		processElevated = originalElevated
	})
	processElevated = func() bool { return true }
	var calls [][]string
	runCertificateDelete = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		if args[0] == "-user" {
			return nil, errors.New("already absent")
		}
		return nil, nil
	}
	fetchCertificatesForCheck = func() ([]Certificate, error) { return nil, nil }
	if err := RemoveLegacyRoot(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || strings.Join(calls[0], " ") != "-delstore Root "+LegacyRootThumbprint || strings.Join(calls[1], " ") != "-user -delstore Root "+LegacyRootThumbprint {
		t.Fatalf("unexpected removal targets: %v", calls)
	}
	fetchCertificatesForCheck = func() ([]Certificate, error) { return []Certificate{{Thumbprint: LegacyRootThumbprint}}, nil }
	if err := RemoveLegacyRoot(); err == nil {
		t.Fatal("reported success while shared root remains")
	}
}

func TestRunHelperIgnoresNormalArguments(t *testing.T) {
	handled, err := RunHelper([]string{"--debug"})
	if handled || err != nil {
		t.Fatalf("RunHelper() = (%v, %v), want not handled", handled, err)
	}
}

func TestRunHelperRejectsInvalidCertificatePayload(t *testing.T) {
	handled, err := RunHelper([]string{installCertArgPrefix + "not-base64"})
	if !handled || err == nil {
		t.Fatalf("RunHelper() = (%v, %v), want handled error", handled, err)
	}
}

func TestRunHelperAcceptsRemovedLegacyRootDespiteAbsentStoreError(t *testing.T) {
	originalDelete, originalFetch := runCertificateDelete, fetchCertificatesForCheck
	t.Cleanup(func() { runCertificateDelete, fetchCertificatesForCheck = originalDelete, originalFetch })
	runCertificateDelete = func(args ...string) ([]byte, error) { return nil, errors.New("already absent") }
	fetchCertificatesForCheck = func() ([]Certificate, error) { return nil, nil }
	if handled, err := RunHelper([]string{removeLegacyArg}); !handled || err != nil {
		t.Fatalf("helper should accept confirmed removal: handled=%v, err=%v", handled, err)
	}
}

func TestUninstallCertificateDeletesCurrentUserStoreBeforeElevation(t *testing.T) {
	originalDelete, originalFetch := runCertificateDelete, fetchCertificatesForCheck
	originalElevated, originalLaunch := processElevated, launchElevated
	t.Cleanup(func() {
		runCertificateDelete, fetchCertificatesForCheck = originalDelete, originalFetch
		processElevated, launchElevated = originalElevated, originalLaunch
	})
	processElevated = func() bool { return false }
	var deleteCalls [][]string
	runCertificateDelete = func(args ...string) ([]byte, error) {
		deleteCalls = append(deleteCalls, args)
		return nil, nil
	}
	var launchedArgs []string
	launchElevated = func(executable string, args []string, directory string) error {
		launchedArgs = append([]string(nil), args...)
		return nil
	}
	fetchCertificatesForCheck = func() ([]Certificate, error) { return nil, nil }

	if err := uninstallCertificate(LegacyRootThumbprint); err != nil {
		t.Fatal(err)
	}
	if len(deleteCalls) != 1 || strings.Join(deleteCalls[0], " ") != "-user -delstore Root "+LegacyRootThumbprint {
		t.Fatalf("current-user store was not cleaned before elevation: %v", deleteCalls)
	}
	if len(launchedArgs) != 1 || launchedArgs[0] != removeLegacyArg {
		t.Fatalf("unexpected elevated helper arguments: %v", launchedArgs)
	}
}

func TestInstallCertificateLaunchesElevatedHelperWithPublicCertificate(t *testing.T) {
	originalElevated, originalLaunch, originalFetch := processElevated, launchElevated, fetchCertificatesForCheck
	t.Cleanup(func() {
		processElevated, launchElevated, fetchCertificatesForCheck = originalElevated, originalLaunch, originalFetch
	})
	processElevated = func() bool { return false }
	cert, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thumbprint, err := Thumbprint(cert.Cert)
	if err != nil {
		t.Fatal(err)
	}
	var launched []string
	launchElevated = func(executable string, args []string, directory string) error {
		launched = append([]string(nil), args...)
		return nil
	}
	fetchCertificatesForCheck = func() ([]Certificate, error) {
		return []Certificate{{Thumbprint: thumbprint}}, nil
	}

	if err := installCertificate(cert.Cert); err != nil {
		t.Fatal(err)
	}
	if len(launched) != 1 || !strings.HasPrefix(launched[0], installCertArgPrefix) {
		t.Fatalf("unexpected helper arguments: %v", launched)
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(launched[0], installCertArgPrefix))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, cert.Cert) {
		t.Fatal("helper received a different certificate payload")
	}
}

func TestVerifyCertificateOperation(t *testing.T) {
	originalFetch := fetchCertificatesForCheck
	t.Cleanup(func() { fetchCertificatesForCheck = originalFetch })
	helperFailure := errors.New("helper exited with code 7")
	checkFailure := errors.New("certificate store unavailable")
	for _, tc := range []struct {
		name             string
		present, matched bool
		helperErr        error
		checkErr         error
		wantErr          bool
	}{
		{name: "installation failed", present: true, helperErr: helperFailure, wantErr: true},
		{name: "removal failed", matched: true, helperErr: helperFailure, wantErr: true},
		{name: "cancelled UAC", present: true, helperErr: windows.ERROR_CANCELLED, wantErr: true},
		{name: "successful exit without installed root", present: true, wantErr: true},
		{name: "successful exit with old root remaining", matched: true, wantErr: true},
		{name: "root removed despite one store error", helperErr: helperFailure},
		{name: "installed root confirmed", present: true, matched: true},
		{name: "failed final check", present: true, helperErr: helperFailure, checkErr: checkFailure, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checks := 0
			fetchCertificatesForCheck = func() ([]Certificate, error) {
				checks++
				if tc.matched {
					return []Certificate{{Thumbprint: LegacyRootThumbprint}}, tc.checkErr
				}
				return nil, tc.checkErr
			}
			err := verifyCertificateOperation(LegacyRootThumbprint, tc.present, tc.helperErr)
			if (err != nil) != tc.wantErr || checks != 1 {
				t.Fatalf("result = %v, checks = %d; want error = %v and one final check", err, checks, tc.wantErr)
			}
			if tc.wantErr && tc.helperErr != nil && !errors.Is(err, tc.helperErr) {
				t.Fatalf("lost helper failure: %v", err)
			}
			if tc.checkErr != nil && !errors.Is(err, tc.checkErr) {
				t.Fatalf("lost certificate store error: %v", err)
			}
		})
	}
}

func TestInstallCertificateReturnsCancelledUACWithoutPolling(t *testing.T) {
	originalElevated, originalLaunch, originalFetch := processElevated, launchElevated, fetchCertificatesForCheck
	t.Cleanup(func() {
		processElevated, launchElevated, fetchCertificatesForCheck = originalElevated, originalLaunch, originalFetch
	})
	processElevated = func() bool { return false }
	launchElevated = func(string, []string, string) error { return windows.ERROR_CANCELLED }
	checks := 0
	fetchCertificatesForCheck = func() ([]Certificate, error) { checks++; return nil, nil }
	cert, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := installCertificate(cert.Cert); !errors.Is(err, windows.ERROR_CANCELLED) || checks != 1 {
		t.Fatalf("installation lost cancellation or kept polling: err=%v, checks=%d", err, checks)
	}
}

func TestWaitForElevatedProcess(t *testing.T) {
	for _, tc := range []struct {
		mode    string
		timeout time.Duration
		want    string
	}{
		{mode: "success", timeout: 10 * time.Second},
		{mode: "failure", timeout: 10 * time.Second, want: "退出码 7"},
		{mode: "sleep", timeout: 25 * time.Millisecond, want: "超时"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestCertificateProcessHelper$")
			cmd.Env = append(os.Environ(), "WX_MINI_VIDEO_CERT_PROCESS_TEST="+tc.mode)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(cmd.Process.Pid))
			if err != nil {
				t.Fatal(err)
			}
			defer windows.CloseHandle(process)
			err = waitForElevatedProcess(process, tc.timeout)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wait result = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCertificateProcessHelper(t *testing.T) {
	switch os.Getenv("WX_MINI_VIDEO_CERT_PROCESS_TEST") {
	case "success":
		os.Exit(0)
	case "failure":
		os.Exit(7)
	case "sleep":
		time.Sleep(time.Minute)
	}
}

func TestShellExecuteInfoStartsOrdinaryProcess(t *testing.T) {
	t.Setenv("WX_MINI_VIDEO_CERT_PROCESS_TEST", "success")
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	parameters, _ := windows.UTF16PtrFromString("-test.run=^TestCertificateProcessHelper$")
	info := shellExecuteInfo{
		mask: 0x00000040 | 0x00000100,
		verb: verb, file: file, parameters: parameters,
	}
	info.size = uint32(unsafe.Sizeof(info))
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	result, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info)))
	if info.process != 0 {
		defer windows.CloseHandle(info.process)
		defer windows.TerminateProcess(info.process, 1)
	}
	if result == 0 || info.process == 0 {
		t.Fatalf("ShellExecuteExW failed with native structure: result=%d, process=%v, err=%v", result, info.process, callErr)
	}
	if err := waitForElevatedProcess(info.process, 10*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestManagedPrivateKeyHasProtectedUserACL(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadOrCreate(dir); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, filepath.Join(dir, "root-ca.pem")} {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		control, _, err := sd.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatal("private key inherits unrestricted ACL")
		}
		text := sd.String()
		if strings.Count(text, "(A;") != 2 || !strings.Contains(text, user.User.Sid.String()) || !strings.Contains(text, ";;;SY)") {
			t.Fatalf("unexpected access control: %s", text)
		}
	}
}

func TestParseCertutilCertificatesChineseOutput(t *testing.T) {
	output := `Root
================ 证书 2 ================
序列号: 0100000000000000000000000000000000
颁发者: CN=SunnyNet, OU=SunnyNet, O=SunnyNet, L=BeiJing, S=BeiJing, C=CN
 NotBefore: 2022-11-04 15:05
 NotAfter: 2122-10-11 15:05
使用者: CN=SunnyNet, OU=SunnyNet, O=SunnyNet, L=BeiJing, S=BeiJing, C=CN
 ""

证书哈希(sha1): d70cd039051f77c30673b8209fc15efa650ed52c

CertUtil: -store 命令成功完成。`
	certificates := parseCertutilCertificates(output)
	if len(certificates) != 1 {
		t.Fatalf("len(parseCertutilCertificates()) = %d, want 1: %#v", len(certificates), certificates)
	}
	if certificates[0].Subject.CN != "SunnyNet" {
		t.Fatalf("CN = %q", certificates[0].Subject.CN)
	}
	if certificates[0].Thumbprint != "D70CD039051F77C30673B8209FC15EFA650ED52C" {
		t.Fatalf("Thumbprint = %q", certificates[0].Thumbprint)
	}
}

func TestParseCertificateSubject(t *testing.T) {
	got := parseCertificateSubject("CN=SunnyNet, OU=SunnyNet, O=SunnyNet, L=BeiJing, S=BeiJing, C=CN")
	if got.CN != "SunnyNet" || got.OU != "SunnyNet" || got.O != "SunnyNet" || got.L != "BeiJing" || got.S != "BeiJing" || got.C != "CN" {
		t.Fatalf("parseCertificateSubject() = %#v", got)
	}
}
