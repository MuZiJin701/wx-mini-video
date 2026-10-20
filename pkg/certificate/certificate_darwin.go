package certificate

func fetchCertificates() ([]Certificate, error)     { return macFetchCertificates() }
func installCertificate(data []byte) error          { return macInstallCertificate(data) }
func uninstallCertificate(fingerprint string) error { return macUninstallCertificate(fingerprint) }
