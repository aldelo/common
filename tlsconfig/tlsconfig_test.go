package tlsconfig

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --------------------------------------------------------------------------
// Test helpers — generate self-signed certs in temp files
// --------------------------------------------------------------------------

// certFiles holds the paths to a generated cert and key pair.
type certFiles struct {
	CertPath string
	KeyPath  string
}

// generateSelfSignedCert creates a self-signed ECDSA certificate and key,
// writes them to PEM files in dir, and returns the file paths.
func generateSelfSignedCert(t *testing.T, dir string, cn string) certFiles {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certPath := filepath.Join(dir, cn+"-cert.pem")
	keyPath := filepath.Join(dir, cn+"-key.pem")

	// Write cert PEM
	certFile, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatalf("encode cert: %v", err)
	}
	certFile.Close()

	// Write key PEM
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyFile, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatalf("encode key: %v", err)
	}
	keyFile.Close()

	return certFiles{CertPath: certPath, KeyPath: keyPath}
}

// writeBadPEM writes a file that is not valid PEM cert data.
func writeBadPEM(t *testing.T, dir string, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("not-a-valid-pem-file"), 0600); err != nil {
		t.Fatalf("write bad pem: %v", err)
	}
	return p
}

// --------------------------------------------------------------------------
// _stringSliceExtractUnique tests
// --------------------------------------------------------------------------

func TestStringSliceExtractUnique_Nil(t *testing.T) {
	result := _stringSliceExtractUnique(nil)
	if result == nil || len(result) != 0 {
		t.Errorf("nil input: expected empty slice, got %v", result)
	}
}

func TestStringSliceExtractUnique_Single(t *testing.T) {
	input := []string{"Abc"}
	result := _stringSliceExtractUnique(input)
	if len(result) != 1 || result[0] != "Abc" {
		t.Errorf("single element: got %v", result)
	}
}

func TestStringSliceExtractUnique_Duplicates(t *testing.T) {
	input := []string{"Abc", "abc", "ABC", "def", "DEF", "ghi"}
	result := _stringSliceExtractUnique(input)
	// Case-insensitive dedup: should keep first occurrence of each.
	if len(result) != 3 {
		t.Fatalf("expected 3 unique, got %d: %v", len(result), result)
	}
	// The first occurrence's original casing should be preserved.
	if result[0] != "Abc" {
		t.Errorf("result[0] = %q, want %q", result[0], "Abc")
	}
	if result[1] != "def" {
		t.Errorf("result[1] = %q, want %q", result[1], "def")
	}
	if result[2] != "ghi" {
		t.Errorf("result[2] = %q, want %q", result[2], "ghi")
	}
}

func TestStringSliceExtractUnique_Empty(t *testing.T) {
	result := _stringSliceExtractUnique([]string{})
	if len(result) != 0 {
		t.Errorf("empty input: expected empty result, got %v", result)
	}
}

func TestStringSliceExtractUnique_NoDuplicates(t *testing.T) {
	input := []string{"alpha", "beta", "gamma"}
	result := _stringSliceExtractUnique(input)
	if len(result) != 3 {
		t.Errorf("expected 3, got %d: %v", len(result), result)
	}
}

// --------------------------------------------------------------------------
// GetServerTlsConfig tests
// --------------------------------------------------------------------------

func TestGetServerTlsConfig_ValidCerts(t *testing.T) {
	dir := t.TempDir()
	server := generateSelfSignedCert(t, dir, "server")

	tc := &TlsConfig{}
	cfg, err := tc.GetServerTlsConfig(server.CertPath, server.KeyPath, nil)
	if err != nil {
		t.Fatalf("GetServerTlsConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", cfg.MinVersion, tls.VersionTLS12)
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(cfg.Certificates))
	}
	// Without client CA certs, should be NoClientCert.
	if cfg.ClientAuth != tls.NoClientCert {
		t.Errorf("ClientAuth = %v, want NoClientCert", cfg.ClientAuth)
	}
}

func TestGetServerTlsConfig_WithClientCA(t *testing.T) {
	dir := t.TempDir()
	server := generateSelfSignedCert(t, dir, "server")
	clientCA := generateSelfSignedCert(t, dir, "client-ca")

	tc := &TlsConfig{}
	cfg, err := tc.GetServerTlsConfig(server.CertPath, server.KeyPath, []string{clientCA.CertPath})
	if err != nil {
		t.Fatalf("GetServerTlsConfig with client CA: %v", err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", cfg.ClientAuth)
	}
	if cfg.ClientCAs == nil {
		t.Error("expected non-nil ClientCAs pool")
	}
}

func TestGetServerTlsConfig_EmptyPaths(t *testing.T) {
	tc := &TlsConfig{}
	_, err := tc.GetServerTlsConfig("", "", nil)
	if err == nil {
		t.Error("expected error with empty cert/key paths")
	}
}

func TestGetServerTlsConfig_InvalidCertPath(t *testing.T) {
	tc := &TlsConfig{}
	_, err := tc.GetServerTlsConfig("/nonexistent/cert.pem", "/nonexistent/key.pem", nil)
	if err == nil {
		t.Error("expected error with nonexistent cert files")
	}
}

func TestGetServerTlsConfig_InvalidClientCAPath(t *testing.T) {
	dir := t.TempDir()
	server := generateSelfSignedCert(t, dir, "server")

	tc := &TlsConfig{}
	_, err := tc.GetServerTlsConfig(server.CertPath, server.KeyPath, []string{"/nonexistent/ca.pem"})
	if err == nil {
		t.Error("expected error with nonexistent client CA file")
	}
}

func TestGetServerTlsConfig_BadClientCAPem(t *testing.T) {
	dir := t.TempDir()
	server := generateSelfSignedCert(t, dir, "server")
	badCA := writeBadPEM(t, dir, "bad-ca.pem")

	tc := &TlsConfig{}
	_, err := tc.GetServerTlsConfig(server.CertPath, server.KeyPath, []string{badCA})
	if err == nil {
		t.Error("expected error with invalid CA PEM data")
	}
}

func TestGetServerTlsConfig_DuplicateClientCAs(t *testing.T) {
	dir := t.TempDir()
	server := generateSelfSignedCert(t, dir, "server")
	clientCA := generateSelfSignedCert(t, dir, "client-ca")

	tc := &TlsConfig{}
	// Pass the same CA twice — dedup should handle it.
	cfg, err := tc.GetServerTlsConfig(server.CertPath, server.KeyPath,
		[]string{clientCA.CertPath, clientCA.CertPath})
	if err != nil {
		t.Fatalf("GetServerTlsConfig duplicate CAs: %v", err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, want RequireAndVerifyClientCert", cfg.ClientAuth)
	}
}

// --------------------------------------------------------------------------
// GetClientTlsConfig tests
// --------------------------------------------------------------------------

func TestGetClientTlsConfig_ValidServerCA(t *testing.T) {
	dir := t.TempDir()
	serverCA := generateSelfSignedCert(t, dir, "server-ca")

	tc := &TlsConfig{}
	cfg, err := tc.GetClientTlsConfig([]string{serverCA.CertPath}, "", "")
	if err != nil {
		t.Fatalf("GetClientTlsConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", cfg.MinVersion, tls.VersionTLS12)
	}
	if cfg.RootCAs == nil {
		t.Error("expected non-nil RootCAs")
	}
	// No client cert set for plain TLS (not mTLS).
	if len(cfg.Certificates) != 0 {
		t.Errorf("expected 0 client certs for plain TLS, got %d", len(cfg.Certificates))
	}
}

func TestGetClientTlsConfig_MTLS(t *testing.T) {
	dir := t.TempDir()
	serverCA := generateSelfSignedCert(t, dir, "server-ca")
	clientCert := generateSelfSignedCert(t, dir, "client")

	tc := &TlsConfig{}
	cfg, err := tc.GetClientTlsConfig([]string{serverCA.CertPath}, clientCert.CertPath, clientCert.KeyPath)
	if err != nil {
		t.Fatalf("GetClientTlsConfig mTLS: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 client cert for mTLS, got %d", len(cfg.Certificates))
	}
}

func TestGetClientTlsConfig_EmptyServerCA(t *testing.T) {
	tc := &TlsConfig{}
	_, err := tc.GetClientTlsConfig(nil, "", "")
	if err == nil {
		t.Error("expected error with nil serverCaCertPemPath")
	}

	_, err = tc.GetClientTlsConfig([]string{}, "", "")
	if err == nil {
		t.Error("expected error with empty serverCaCertPemPath")
	}
}

func TestGetClientTlsConfig_InvalidServerCAPath(t *testing.T) {
	tc := &TlsConfig{}
	_, err := tc.GetClientTlsConfig([]string{"/nonexistent/server-ca.pem"}, "", "")
	if err == nil {
		t.Error("expected error with nonexistent server CA path")
	}
}

func TestGetClientTlsConfig_BadServerCAPem(t *testing.T) {
	dir := t.TempDir()
	badCA := writeBadPEM(t, dir, "bad-server-ca.pem")

	tc := &TlsConfig{}
	_, err := tc.GetClientTlsConfig([]string{badCA}, "", "")
	if err == nil {
		t.Error("expected error with invalid server CA PEM data")
	}
}

func TestGetClientTlsConfig_InvalidClientCertPath(t *testing.T) {
	dir := t.TempDir()
	serverCA := generateSelfSignedCert(t, dir, "server-ca")

	tc := &TlsConfig{}
	_, err := tc.GetClientTlsConfig([]string{serverCA.CertPath}, "/nonexistent/client.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("expected error with nonexistent client cert/key paths")
	}
}

// --------------------------------------------------------------------------
// CipherSuites verification
// --------------------------------------------------------------------------

func TestGetServerTlsConfig_CipherSuites(t *testing.T) {
	dir := t.TempDir()
	server := generateSelfSignedCert(t, dir, "server")

	tc := &TlsConfig{}
	cfg, err := tc.GetServerTlsConfig(server.CertPath, server.KeyPath, nil)
	if err != nil {
		t.Fatalf("GetServerTlsConfig: %v", err)
	}

	expected := map[uint16]bool{
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:       true,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:       true,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256: true,
	}
	if len(cfg.CipherSuites) != len(expected) {
		t.Fatalf("expected %d cipher suites, got %d", len(expected), len(cfg.CipherSuites))
	}
	for _, cs := range cfg.CipherSuites {
		if !expected[cs] {
			t.Errorf("unexpected cipher suite: %d", cs)
		}
	}
}

func TestGetServerTlsConfig_CurvePreferences(t *testing.T) {
	dir := t.TempDir()
	server := generateSelfSignedCert(t, dir, "server")

	tc := &TlsConfig{}
	cfg, err := tc.GetServerTlsConfig(server.CertPath, server.KeyPath, nil)
	if err != nil {
		t.Fatalf("GetServerTlsConfig: %v", err)
	}

	expectedCurves := []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256}
	if len(cfg.CurvePreferences) != len(expectedCurves) {
		t.Fatalf("expected %d curves, got %d", len(expectedCurves), len(cfg.CurvePreferences))
	}
	for i, c := range expectedCurves {
		if cfg.CurvePreferences[i] != c {
			t.Errorf("CurvePreferences[%d] = %v, want %v", i, cfg.CurvePreferences[i], c)
		}
	}
}
