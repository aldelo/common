package kms

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
	"github.com/aws/aws-xray-sdk-go/xraylog"
)

// syncBuffer is a concurrency safe io.Writer, needed because the xray sdk
// emits from the handler goroutines that wrap each aws sdk request.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// fakeKMSEndpoint serves the two api calls ECDH makes, so the test needs
// neither aws credentials nor network access.
func fakeKMSEndpoint(t *testing.T) *httptest.Server {
	t.Helper()

	const eccPublicKeyB64 = "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE"

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.Header.Get("X-Amz-Target")
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")

		switch {
		case strings.HasSuffix(target, "DescribeKey"):
			_, _ = w.Write([]byte(`{"KeyMetadata":{"KeyId":"test-key","KeySpec":"ECC_NIST_P256",` +
				`"KeyUsage":"KEY_AGREEMENT","KeyState":"Enabled"}}`))
		case strings.HasSuffix(target, "DeriveSharedSecret"):
			_, _ = w.Write([]byte(`{"SharedSecret":"` +
				base64.StdEncoding.EncodeToString([]byte("0123456789abcdef")) + `"}`))
		default:
			t.Errorf("unexpected X-Amz-Target: %q", target)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

// TestECDH_DoesNotEmitXrayContextMissing pins the fix for ECDH having been the
// only method in this package that opened no segment and used the plain (non
// *WithContext) sdk calls. Connect() instruments the shared client with
// awsxray.AWS(), so those calls reached the xray handlers on a background
// context and each emitted three "segment cannot be found" records for the
// 'kms', 'attempt' and 'unmarshal' subsegments -- which the default
// RUNTIME_ERROR context missing strategy turns into a panic.
func TestECDH_DoesNotEmitXrayContextMissing(t *testing.T) {
	t.Setenv("AWS_XRAY_CONTEXT_MISSING", "LOG_ERROR")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")

	srv := fakeKMSEndpoint(t)
	defer srv.Close()

	if err := xray.Init("127.0.0.1:2000", "1.2.0"); err != nil {
		t.Fatalf("xray.Init: %v", err)
	}
	if !xray.XRayServiceOn() {
		t.Skip("xray tracing is disabled in this environment")
	}

	logs := &syncBuffer{}
	awsxray.SetLogger(xraylog.NewDefaultLogger(logs, xraylog.LogLevelError))

	k := &KMS{
		AwsRegion:      awsregion.AWS_us_east_1_nvirginia,
		CustomEndpoint: srv.URL,
	}
	if err := k.Connect(nil); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer k.Disconnect()

	sharedSecret, err := k.ECDH("test-key", "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE")
	if err != nil {
		t.Fatalf("ECDH: %v", err)
	}
	if len(sharedSecret) == 0 {
		t.Fatal("ECDH returned an empty shared secret")
	}

	if out := logs.String(); strings.Contains(out, "segment cannot be found") {
		t.Errorf("ECDH emitted xray context missing records:\n%s", out)
	}
}
