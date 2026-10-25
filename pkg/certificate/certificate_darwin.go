package certificate

func fetchCertificates() ([]Certificate, error)     { return macFetchCertificates() }
func installCertificate(data []byte) error          { return macInstallCertificate(data) }
func uninstallCertificate(fingerprint string) error { return macUninstallCertificate(fingerprint) }

// RunHelper 仅用于 Windows 提权子进程；macOS 不通过该路径安装证书。
func RunHelper(args []string) (bool, error) { return false, nil }
