package rest

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestGETSuccess verifies a successful GET request returns status 200 and
// the expected body.
func TestGETSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	statusCode, body, err := GET(server.URL, nil)
	if err != nil {
		t.Fatalf("GET() returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("GET() statusCode = %d, expected %d", statusCode, http.StatusOK)
	}
	if body != `{"status":"ok"}` {
		t.Errorf("GET() body = %q, expected %q", body, `{"status":"ok"}`)
	}
}

// TestGETWithCustomHeaders verifies that custom headers are sent with the request.
func TestGETWithCustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization header = %q, expected %q", auth, "Bearer test-token")
		}
		custom := r.Header.Get("X-Custom-Header")
		if custom != "custom-value" {
			t.Errorf("X-Custom-Header = %q, expected %q", custom, "custom-value")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	headers := []*HeaderKeyValue{
		{Key: "Authorization", Value: "Bearer test-token"},
		{Key: "X-Custom-Header", Value: "custom-value"},
	}

	statusCode, _, err := GET(server.URL, headers)
	if err != nil {
		t.Fatalf("GET() with headers returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("GET() statusCode = %d, expected %d", statusCode, http.StatusOK)
	}
}

// TestGETNon200Response verifies that a non-200 response returns an error.
func TestGETNon200Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	statusCode, _, err := GET(server.URL, nil)
	if err == nil {
		t.Fatal("GET() should return error for 404 response")
	}
	if statusCode != http.StatusNotFound {
		t.Errorf("GET() statusCode = %d, expected %d", statusCode, http.StatusNotFound)
	}
}

// TestPOSTWithJSONBody verifies a POST request with a JSON body.
func TestPOSTWithJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, expected %q", ct, "application/json")
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		body := string(bodyBytes)
		if !strings.Contains(body, `"name":"test"`) {
			t.Errorf("Request body = %q, expected to contain '\"name\":\"test\"'", body)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer server.Close()

	headers := []*HeaderKeyValue{
		{Key: "Content-Type", Value: "application/json"},
	}

	statusCode, body, err := POST(server.URL, headers, `{"name":"test"}`)
	if err != nil {
		t.Fatalf("POST() returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("POST() statusCode = %d, expected %d", statusCode, http.StatusOK)
	}
	if body != `{"id":1}` {
		t.Errorf("POST() body = %q, expected %q", body, `{"id":1}`)
	}
}

// TestPOSTDefaultContentType verifies that POST sets default Content-Type
// to application/x-www-form-urlencoded when no Content-Type header is provided.
func TestPOSTDefaultContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, expected %q", ct, "application/x-www-form-urlencoded")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	statusCode, _, err := POST(server.URL, nil, "key=value")
	if err != nil {
		t.Fatalf("POST() returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("POST() statusCode = %d, expected %d", statusCode, http.StatusOK)
	}
}

// TestPUTSuccess verifies a successful PUT request.
func TestPUTSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT method, got %s", r.Method)
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		if string(bodyBytes) != `{"update":"data"}` {
			t.Errorf("Request body = %q, expected %q", string(bodyBytes), `{"update":"data"}`)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"updated":true}`))
	}))
	defer server.Close()

	headers := []*HeaderKeyValue{
		{Key: "Content-Type", Value: "application/json"},
	}

	statusCode, body, err := PUT(server.URL, headers, `{"update":"data"}`)
	if err != nil {
		t.Fatalf("PUT() returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("PUT() statusCode = %d, expected %d", statusCode, http.StatusOK)
	}
	if body != `{"updated":true}` {
		t.Errorf("PUT() body = %q, expected %q", body, `{"updated":true}`)
	}
}

// TestDELETESuccess verifies a successful DELETE request.
func TestDELETESuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"deleted":true}`))
	}))
	defer server.Close()

	statusCode, body, err := DELETE(server.URL, nil)
	if err != nil {
		t.Fatalf("DELETE() returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("DELETE() statusCode = %d, expected %d", statusCode, http.StatusOK)
	}
	if body != `{"deleted":true}` {
		t.Errorf("DELETE() body = %q, expected %q", body, `{"deleted":true}`)
	}
}

// TestDELETENon200Response verifies DELETE with non-200 response.
func TestDELETENon200Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	statusCode, _, err := DELETE(server.URL, nil)
	if err == nil {
		t.Fatal("DELETE() should return error for 403 response")
	}
	if statusCode != http.StatusForbidden {
		t.Errorf("DELETE() statusCode = %d, expected %d", statusCode, http.StatusForbidden)
	}
}

// TestPOSTServerError verifies POST with a 500 server error response.
func TestPOSTServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	statusCode, _, err := POST(server.URL, nil, "data")
	if err == nil {
		t.Fatal("POST() should return error for 500 response")
	}
	if statusCode != http.StatusInternalServerError {
		t.Errorf("POST() statusCode = %d, expected %d", statusCode, http.StatusInternalServerError)
	}
}

// TestGETInvalidURL verifies GET with an invalid URL returns an error.
func TestGETInvalidURL(t *testing.T) {
	_, _, err := GET("http://invalid-host-that-does-not-exist.local:99999/path", nil)
	if err == nil {
		t.Error("GET() should return error for invalid URL")
	}
}

// TestPUTDefaultContentType verifies that PUT sets default Content-Type header.
func TestPUTDefaultContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, expected %q", ct, "application/x-www-form-urlencoded")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	statusCode, _, err := PUT(server.URL, nil, "data")
	if err != nil {
		t.Fatalf("PUT() returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("PUT() statusCode = %d, expected %d", statusCode, http.StatusOK)
	}
}

// TestGETEmptyBody verifies GET with a 200 response and empty body.
func TestGETEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	statusCode, body, err := GET(server.URL, nil)
	if err != nil {
		t.Fatalf("GET() returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("GET() statusCode = %d, expected %d", statusCode, http.StatusOK)
	}
	if body != "" {
		t.Errorf("GET() body = %q, expected empty string", body)
	}
}

// TestHeaderKeyValueStruct verifies HeaderKeyValue struct fields.
func TestHeaderKeyValueStruct(t *testing.T) {
	h := &HeaderKeyValue{
		Key:   "Authorization",
		Value: "Bearer abc123",
	}

	if h.Key != "Authorization" {
		t.Errorf("Key = %q, expected %q", h.Key, "Authorization")
	}
	if h.Value != "Bearer abc123" {
		t.Errorf("Value = %q, expected %q", h.Value, "Bearer abc123")
	}
}

// TestSetClientTimeoutSeconds verifies that SetClientTimeoutSeconds does not panic.
func TestSetClientTimeoutSeconds(t *testing.T) {
	// Save and restore original value
	mu.RLock()
	origTimeout := clientTimeoutSeconds
	mu.RUnlock()

	defer func() {
		mu.Lock()
		clientTimeoutSeconds = origTimeout
		mu.Unlock()
	}()

	SetClientTimeoutSeconds(60)

	mu.RLock()
	got := clientTimeoutSeconds
	mu.RUnlock()

	if got != 60 {
		t.Errorf("clientTimeoutSeconds = %d, expected 60", got)
	}
}

// TestResetServerCAPemFiles verifies that ResetServerCAPemFiles with empty
// args clears the config without error.
func TestResetServerCAPemFiles(t *testing.T) {
	err := ResetServerCAPemFiles()
	if err != nil {
		t.Errorf("ResetServerCAPemFiles() returned error: %v", err)
	}
}

// TestGETWithDELETEHeaders verifies that DELETE passes custom headers correctly.
func TestDELETEWithCustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer delete-token" {
			t.Errorf("Authorization = %q, expected %q", auth, "Bearer delete-token")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("deleted"))
	}))
	defer server.Close()

	headers := []*HeaderKeyValue{
		{Key: "Authorization", Value: "Bearer delete-token"},
	}

	statusCode, _, err := DELETE(server.URL, headers)
	if err != nil {
		t.Fatalf("DELETE() returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("DELETE() statusCode = %d, expected %d", statusCode, http.StatusOK)
	}
}

// TestCloneClientTlsConfigReturnsNilWhenNil verifies that cloneClientTlsConfig
// returns nil when no TLS config has been set.
func TestCloneClientTlsConfigReturnsNilWhenNil(t *testing.T) {
	// Save and restore original value
	mu.Lock()
	origConfig := clientTlsConfig
	clientTlsConfig = nil
	mu.Unlock()

	defer func() {
		mu.Lock()
		clientTlsConfig = origConfig
		mu.Unlock()
	}()

	mu.RLock()
	got := cloneClientTlsConfig()
	mu.RUnlock()

	if got != nil {
		t.Error("cloneClientTlsConfig() should return nil when clientTlsConfig is nil")
	}
}

// TestCloneClientTlsConfigIsolation verifies that mutating the cloned TLS config
// does NOT affect the process-global clientTlsConfig.
// This is the core test for AD-2: without Clone(), modifying the returned config
// would corrupt the shared state for all consumers in the process.
func TestCloneClientTlsConfigIsolation(t *testing.T) {
	// Save and restore original value
	mu.Lock()
	origConfig := clientTlsConfig
	mu.Unlock()

	defer func() {
		mu.Lock()
		clientTlsConfig = origConfig
		mu.Unlock()
	}()

	// Set a known global TLS config
	mu.Lock()
	clientTlsConfig = &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         "original.example.com",
		MinVersion:         tls.VersionTLS12,
	}
	mu.Unlock()

	// Get a clone
	mu.RLock()
	cloned := cloneClientTlsConfig()
	mu.RUnlock()

	if cloned == nil {
		t.Fatal("cloneClientTlsConfig() returned nil, expected a clone")
	}

	// Verify the clone has the same values
	if cloned.ServerName != "original.example.com" {
		t.Errorf("cloned ServerName = %q, expected %q", cloned.ServerName, "original.example.com")
	}
	if cloned.InsecureSkipVerify != false {
		t.Error("cloned InsecureSkipVerify should be false")
	}

	// Mutate the clone — this simulates what http.Transport or a consumer might do
	cloned.InsecureSkipVerify = true
	cloned.ServerName = "mutated.example.com"
	cloned.MinVersion = tls.VersionTLS13

	// Verify the global config is NOT affected by the mutation
	mu.RLock()
	if clientTlsConfig.InsecureSkipVerify != false {
		t.Error("Global clientTlsConfig.InsecureSkipVerify was mutated by clone modification — isolation failure")
	}
	if clientTlsConfig.ServerName != "original.example.com" {
		t.Errorf("Global clientTlsConfig.ServerName = %q, was mutated by clone — isolation failure", clientTlsConfig.ServerName)
	}
	if clientTlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Global clientTlsConfig.MinVersion = %d, was mutated by clone — isolation failure", clientTlsConfig.MinVersion)
	}
	mu.RUnlock()
}

// TestGET_ConnectionReuseAcrossCalls_P1PERF1 verifies that sequential GET calls
// reuse the same TCP connection via the shared *http.Transport. The previous
// implementation constructed a new Transport per call and then called
// CloseIdleConnections, forcing a fresh TCP+TLS handshake per request — which
// this test would catch by observing one StateNew connection per GET.
//
// Expectation: 10 GETs against the same host should open at most 2 TCP
// connections (1 expected under ideal keep-alive; allow 1 extra for scheduler
// races in the test harness). With the unpooled pre-v1.8.8 code, newConns == 10.
func TestGET_ConnectionReuseAcrossCalls_P1PERF1(t *testing.T) {
	var newConns int64

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			atomic.AddInt64(&newConns, 1)
		}
	}
	server.Start()
	defer server.Close()

	// Reset the shared transport so TLS-config invalidation from prior tests
	// can't leak into this test. Then prime it with the first call.
	mu.Lock()
	sharedTransport = nil
	mu.Unlock()

	const calls = 10
	for i := 0; i < calls; i++ {
		statusCode, _, err := GET(server.URL, nil)
		if err != nil {
			t.Fatalf("GET #%d returned error: %v", i, err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("GET #%d statusCode = %d, expected 200", i, statusCode)
		}
	}

	got := atomic.LoadInt64(&newConns)
	if got > 2 {
		t.Errorf("connection reuse broken: %d TCP connections opened across %d sequential GETs (expected <=2). "+
			"This indicates the shared Transport is being discarded or CloseIdleConnections is being called.",
			got, calls)
	}
}

// TestSharedTransport_SingletonAcrossCalls_P1PERF1 verifies that getSharedTransport
// returns the SAME *http.Transport pointer across calls — proving Transport state
// (idle conn pool, TLS session cache) is actually shared.
func TestSharedTransport_SingletonAcrossCalls_P1PERF1(t *testing.T) {
	mu.Lock()
	sharedTransport = nil
	mu.Unlock()

	first := getSharedTransport()
	if first == nil {
		t.Fatal("getSharedTransport() returned nil on first call")
	}

	second := getSharedTransport()
	if second != first {
		t.Errorf("getSharedTransport() returned a different pointer on second call: first=%p second=%p — singleton broken", first, second)
	}
}

// TestSharedTransport_RebuildAfterTlsConfigChange_P1PERF1 verifies that
// AppendServerCAPemFiles / ResetServerCAPemFiles invalidate sharedTransport
// so subsequent calls pick up the new TLS config. Without invalidation, a
// config refresh would be silently ignored for every pooled connection.
func TestSharedTransport_RebuildAfterTlsConfigChange_P1PERF1(t *testing.T) {
	mu.Lock()
	sharedTransport = nil
	origConfig := clientTlsConfig
	mu.Unlock()

	defer func() {
		mu.Lock()
		clientTlsConfig = origConfig
		sharedTransport = nil
		mu.Unlock()
	}()

	first := getSharedTransport()

	// Simulate a TLS config change the same way ResetServerCAPemFiles does.
	if err := ResetServerCAPemFiles(); err != nil {
		t.Fatalf("ResetServerCAPemFiles() returned error: %v", err)
	}

	mu.RLock()
	invalidated := sharedTransport
	mu.RUnlock()
	if invalidated != nil {
		t.Error("ResetServerCAPemFiles did NOT invalidate sharedTransport — next call would use stale TLS config")
	}

	second := getSharedTransport()
	if second == first {
		t.Error("getSharedTransport() returned the same Transport after TLS config change — rebuild did not happen")
	}
}

// TestCloneClientTlsConfigIsDistinctPointer verifies the clone is a different
// pointer from the global, ensuring no aliasing.
func TestCloneClientTlsConfigIsDistinctPointer(t *testing.T) {
	mu.Lock()
	origConfig := clientTlsConfig
	clientTlsConfig = &tls.Config{ServerName: "distinct-test.example.com"}
	mu.Unlock()

	defer func() {
		mu.Lock()
		clientTlsConfig = origConfig
		mu.Unlock()
	}()

	mu.RLock()
	cloned := cloneClientTlsConfig()
	global := clientTlsConfig
	mu.RUnlock()

	if cloned == global {
		t.Error("cloneClientTlsConfig() returned the same pointer as the global — no isolation")
	}
}
