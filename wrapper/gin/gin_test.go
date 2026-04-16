package gin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	ginlib "github.com/gin-gonic/gin"

	"github.com/aldelo/common/wrapper/gin/ginbindtype"
	"github.com/aldelo/common/wrapper/gin/gingzipcompression"
	"github.com/aldelo/common/wrapper/gin/ginhttpmethod"
)

func init() {
	ginlib.SetMode(ginlib.TestMode)
}

// --------------------------------------------------------------------------
// jsonEscapeString tests (unexported pure function)
// --------------------------------------------------------------------------

func TestJsonEscapeString_Plain(t *testing.T) {
	got := jsonEscapeString("hello world")
	if got != "hello world" {
		t.Errorf("plain string: got %q, want %q", got, "hello world")
	}
}

func TestJsonEscapeString_Quotes(t *testing.T) {
	got := jsonEscapeString(`say "hello"`)
	want := `say \"hello\"`
	if got != want {
		t.Errorf("quotes: got %q, want %q", got, want)
	}
}

func TestJsonEscapeString_Backslash(t *testing.T) {
	got := jsonEscapeString(`path\to\file`)
	want := `path\\to\\file`
	if got != want {
		t.Errorf("backslash: got %q, want %q", got, want)
	}
}

func TestJsonEscapeString_Newline(t *testing.T) {
	got := jsonEscapeString("line1\nline2")
	want := `line1\nline2`
	if got != want {
		t.Errorf("newline: got %q, want %q", got, want)
	}
}

func TestJsonEscapeString_Tab(t *testing.T) {
	got := jsonEscapeString("col1\tcol2")
	want := `col1\tcol2`
	if got != want {
		t.Errorf("tab: got %q, want %q", got, want)
	}
}

func TestJsonEscapeString_Empty(t *testing.T) {
	got := jsonEscapeString("")
	if got != "" {
		t.Errorf("empty string: got %q, want empty", got)
	}
}

func TestJsonEscapeString_Unicode(t *testing.T) {
	got := jsonEscapeString("caf\u00e9")
	// json.Marshal preserves valid UTF-8 without escaping.
	if got != "caf\u00e9" {
		t.Errorf("unicode: got %q, want %q", got, "caf\u00e9")
	}
}

func TestJsonEscapeString_ControlChars(t *testing.T) {
	// Control characters like \x00 should be escaped.
	got := jsonEscapeString("a\x00b")
	want := `a\u0000b`
	if got != want {
		t.Errorf("control char: got %q, want %q", got, want)
	}
}

func TestJsonEscapeString_HTMLChars(t *testing.T) {
	got := jsonEscapeString("<script>alert('xss')</script>")
	// json.Marshal escapes < > & by default.
	if got == "<script>alert('xss')</script>" {
		t.Error("HTML chars should be escaped by json.Marshal")
	}
}

// --------------------------------------------------------------------------
// BindPostDataFailed tests
// --------------------------------------------------------------------------

func TestBindPostDataFailed(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)

	BindPostDataFailed(c)

	if w.Code != 412 {
		t.Errorf("status = %d, want 412", w.Code)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty body")
	}
}

func TestBindPostDataFailed_NilContext(t *testing.T) {
	// Should not panic with nil context.
	BindPostDataFailed(nil)
}

// --------------------------------------------------------------------------
// ActionServerFailed tests
// --------------------------------------------------------------------------

func TestActionServerFailed(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)

	ActionServerFailed(c, "db timeout")

	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("db timeout")) {
		t.Errorf("body should contain error message, got %q", body)
	}
	if !bytes.Contains([]byte(body), []byte("action-server-failed")) {
		t.Errorf("body should contain error type, got %q", body)
	}
}

func TestActionServerFailed_EscapesInput(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)

	// Inject a string with JSON-breaking characters.
	ActionServerFailed(c, `error with "quotes" and \backslash`)

	body := w.Body.String()
	// The body should be valid — quotes escaped.
	if bytes.Contains([]byte(body), []byte(`"quotes"`)) {
		t.Errorf("unescaped quotes in body: %q", body)
	}
}

func TestActionServerFailed_NilContext(t *testing.T) {
	ActionServerFailed(nil, "err")
}

// --------------------------------------------------------------------------
// ActionStatusNotOK tests
// --------------------------------------------------------------------------

func TestActionStatusNotOK(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)

	ActionStatusNotOK(c, "not found info")

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("not found info")) {
		t.Errorf("body should contain error info, got %q", body)
	}
}

func TestActionStatusNotOK_NilContext(t *testing.T) {
	ActionStatusNotOK(nil, "err")
}

// --------------------------------------------------------------------------
// MarshalQueryParametersFailed tests
// --------------------------------------------------------------------------

func TestMarshalQueryParametersFailed(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)

	MarshalQueryParametersFailed(c, "bad params")

	if w.Code != 412 {
		t.Errorf("status = %d, want 412", w.Code)
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("marshal-query-parameters")) {
		t.Errorf("body should contain error type, got %q", body)
	}
}

// --------------------------------------------------------------------------
// VerifyGoogleReCAPTCHAv2Failed tests
// --------------------------------------------------------------------------

func TestVerifyGoogleReCAPTCHAv2Failed(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)

	VerifyGoogleReCAPTCHAv2Failed(c, "captcha invalid")

	if w.Code != 412 {
		t.Errorf("status = %d, want 412", w.Code)
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("verify-google-recaptcha-v2")) {
		t.Errorf("body should contain error type, got %q", body)
	}
}

// --------------------------------------------------------------------------
// ResponseBodyWriterInterceptor tests
// --------------------------------------------------------------------------

func TestResponseBodyWriterInterceptor_Write(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)

	buf := &bytes.Buffer{}
	interceptor := &ResponseBodyWriterInterceptor{
		ResponseWriter: c.Writer,
		RespBody:       buf,
	}

	data := []byte("intercepted body")
	n, err := interceptor.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write n = %d, want %d", n, len(data))
	}

	// The interceptor should capture to its buffer.
	if buf.String() != "intercepted body" {
		t.Errorf("interceptor buffer = %q, want %q", buf.String(), "intercepted body")
	}
	// And the original writer should also receive the data.
	if w.Body.String() != "intercepted body" {
		t.Errorf("recorder body = %q, want %q", w.Body.String(), "intercepted body")
	}
}

func TestResponseBodyWriterInterceptor_WriteString(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)

	buf := &bytes.Buffer{}
	interceptor := &ResponseBodyWriterInterceptor{
		ResponseWriter: c.Writer,
		RespBody:       buf,
	}

	s := "string body"
	n, err := interceptor.WriteString(s)
	if err != nil {
		t.Fatalf("WriteString error: %v", err)
	}
	if n != len(s) {
		t.Errorf("WriteString n = %d, want %d", n, len(s))
	}
	if buf.String() != s {
		t.Errorf("interceptor buffer = %q, want %q", buf.String(), s)
	}
	if w.Body.String() != s {
		t.Errorf("recorder body = %q, want %q", w.Body.String(), s)
	}
}

// --------------------------------------------------------------------------
// GZipConfig tests (pure methods, no external deps)
// --------------------------------------------------------------------------

func TestGZipConfig_GetGZipCompression(t *testing.T) {
	tests := []struct {
		name        string
		compression gingzipcompression.GinGZipCompression
		wantNonZero bool // We just verify it doesn't panic and returns a value.
	}{
		{"Default", gingzipcompression.Default, true},
		{"BestCompression", gingzipcompression.BestCompression, true},
		{"BestSpeed", gingzipcompression.BestSpeed, true},
		{"UNKNOWN (no compression)", gingzipcompression.UNKNOWN, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gz := &GZipConfig{Compression: tc.compression}
			val := gz.GetGZipCompression()
			// NoCompression = 0, others are non-zero.
			if tc.wantNonZero && val == 0 {
				t.Errorf("expected non-zero compression value for %s", tc.name)
			}
		})
	}
}

func TestGZipConfig_GetGZipExcludedExtensions(t *testing.T) {
	// With extensions set.
	gz := &GZipConfig{ExcludedExtensions: []string{".png", ".gif"}}
	opt := gz.GetGZipExcludedExtensions()
	if opt == nil {
		t.Error("expected non-nil option when extensions are set")
	}

	// Without extensions.
	gz2 := &GZipConfig{}
	opt2 := gz2.GetGZipExcludedExtensions()
	if opt2 != nil {
		t.Error("expected nil option when no extensions")
	}
}

func TestGZipConfig_GetGZipExcludedPaths(t *testing.T) {
	gz := &GZipConfig{ExcludedPaths: []string{"/api/health"}}
	opt := gz.GetGZipExcludedPaths()
	if opt == nil {
		t.Error("expected non-nil option when paths are set")
	}

	gz2 := &GZipConfig{}
	opt2 := gz2.GetGZipExcludedPaths()
	if opt2 != nil {
		t.Error("expected nil option when no paths")
	}
}

func TestGZipConfig_GetGZipExcludedPathsRegex(t *testing.T) {
	gz := &GZipConfig{ExcludedPathsRegex: []string{`^/debug/.*`}}
	opt := gz.GetGZipExcludedPathsRegex()
	if opt == nil {
		t.Error("expected non-nil option when regex paths are set")
	}

	gz2 := &GZipConfig{}
	opt2 := gz2.GetGZipExcludedPathsRegex()
	if opt2 != nil {
		t.Error("expected nil option when no regex paths")
	}
}

// --------------------------------------------------------------------------
// VerifyGoogleReCAPTCHAv2 edge cases (no external calls)
// --------------------------------------------------------------------------

func TestVerifyGoogleReCAPTCHAv2_NilContext(t *testing.T) {
	err := VerifyGoogleReCAPTCHAv2(nil, "token", true)
	if err == nil {
		t.Error("expected error with nil context")
	}
}

func TestVerifyGoogleReCAPTCHAv2_NotRequired_EmptyResponse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)

	// When not required and response is empty, should return nil (no error).
	err := VerifyGoogleReCAPTCHAv2(c, "", false)
	if err != nil {
		t.Errorf("expected nil error when not required and empty response, got: %v", err)
	}
}

func TestVerifyGoogleReCAPTCHAv2_Required_EmptyResponse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)

	err := VerifyGoogleReCAPTCHAv2(c, "", true)
	if err == nil {
		t.Error("expected error when required and empty response")
	}
}

func TestVerifyGoogleReCAPTCHAv2_Required_NoSecretInContext(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)

	// No "google_recaptcha_secret" set in context — should error when required.
	err := VerifyGoogleReCAPTCHAv2(c, "some-response-token", true)
	if err == nil {
		t.Error("expected error when recaptcha required but secret missing from context")
	}
}

func TestVerifyGoogleReCAPTCHAv2_NotRequired_NoSecretInContext(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)

	// Not required + no secret in context = should return nil.
	err := VerifyGoogleReCAPTCHAv2(c, "some-response-token", false)
	if err != nil {
		t.Errorf("expected nil error when not required and no secret, got: %v", err)
	}
}

func TestVerifyGoogleReCAPTCHAv2_SecretWrongType(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)
	// Set the secret to a non-string type.
	c.Set("google_recaptcha_secret", 12345)

	err := VerifyGoogleReCAPTCHAv2(c, "some-response-token", true)
	if err == nil {
		t.Error("expected error when secret is not a string")
	}
}

// --------------------------------------------------------------------------
// HandleReCAPTCHAv2 edge cases
// --------------------------------------------------------------------------

func TestHandleReCAPTCHAv2_NilContext(t *testing.T) {
	result := HandleReCAPTCHAv2(nil, &struct{}{})
	if result {
		t.Error("expected false with nil context")
	}
}

func TestHandleReCAPTCHAv2_NilBinding(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)

	result := HandleReCAPTCHAv2(c, nil)
	if result {
		t.Error("expected false with nil bindingInputPtr")
	}
}

func TestHandleReCAPTCHAv2_NonReCAPTCHAInterface(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := ginlib.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)

	// Pass a struct that does NOT implement ReCAPTCHAResponseIFace.
	result := HandleReCAPTCHAv2(c, &struct{ Name string }{Name: "test"})
	if result {
		t.Error("expected false when bindingInputPtr does not implement ReCAPTCHAResponseIFace")
	}
	if w.Code != 412 {
		t.Errorf("expected 412, got %d", w.Code)
	}
}

// --------------------------------------------------------------------------
// NiceRecovery tests
// --------------------------------------------------------------------------

func TestNiceRecovery_NoPanic(t *testing.T) {
	router := ginlib.New()
	var recoveredErr interface{}
	router.Use(NiceRecovery(func(c *ginlib.Context, err interface{}) {
		recoveredErr = err
		c.String(500, "recovered")
	}))
	router.GET("/ok", func(c *ginlib.Context) {
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ok", nil)
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if recoveredErr != nil {
		t.Error("recovery handler should not have been called")
	}
}

func TestNiceRecovery_WithPanic(t *testing.T) {
	router := ginlib.New()
	recovered := make(chan interface{}, 1)
	router.Use(NiceRecovery(func(c *ginlib.Context, err interface{}) {
		recovered <- err
		c.String(500, "recovered")
	}))
	router.GET("/panic", func(c *ginlib.Context) {
		panic("test panic")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)
	router.ServeHTTP(w, req)

	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}

	select {
	case err := <-recovered:
		if err == nil {
			t.Error("expected non-nil recovered error")
		}
	default:
		t.Error("recovery handler was not called")
	}
}

// --------------------------------------------------------------------------
// Gin struct — NewServer basic construction tests
// --------------------------------------------------------------------------

func TestNewServer_CreatesEngine(t *testing.T) {
	gw := NewServer("test-server", 0, false, false, nil)
	if gw == nil {
		t.Fatal("NewServer returned nil")
	}
	if gw.Name != "test-server" {
		t.Errorf("Name = %q, want %q", gw.Name, "test-server")
	}
	if gw.Engine() == nil {
		t.Error("expected non-nil gin engine")
	}
}

func TestNewServer_CustomRecovery(t *testing.T) {
	gw := NewServer("test-cr", 0, true, true, func(status int, trace string, c *ginlib.Context) {
		c.String(status, "custom error")
	})
	if gw == nil {
		t.Fatal("NewServer returned nil")
	}
	if gw.Engine() == nil {
		t.Error("expected non-nil gin engine with custom recovery")
	}
}

// TestNewServer_CustomRecovery_NoDetailLeak verifies SR-NEW-1: when a handler
// panics and no HttpStatusErrorHandler is set, the response body must contain
// only "Internal Server Error" — never the raw panic value, stack frames,
// or file paths.
func TestNewServer_CustomRecovery_NoDetailLeak(t *testing.T) {
	gw := NewServer("test-no-leak", 0, true, true, nil)
	if gw == nil {
		t.Fatal("NewServer returned nil")
	}

	// Register a route that panics with sensitive info.
	gw.Engine().GET("/panic-leak", func(c *ginlib.Context) {
		panic("secret: password=hunter2 at /home/user/.config/db.json")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic-leak", nil)
	gw.Engine().ServeHTTP(w, req)

	if w.Code != 500 {
		t.Fatalf("status = %d, want 500", w.Code)
	}

	body := w.Body.String()

	// Must contain the generic message.
	if body != "Internal Server Error" {
		t.Errorf("response body = %q, want %q", body, "Internal Server Error")
	}

	// Must NOT contain any of the panic's sensitive content.
	for _, leak := range []string{"password", "hunter2", "db.json", "/home/user", "secret"} {
		if bytes.Contains(w.Body.Bytes(), []byte(leak)) {
			t.Errorf("response body contains sensitive data %q — panic detail leaked to client", leak)
		}
	}
}

// --------------------------------------------------------------------------
// clientKeyFromRequest — must use c.ClientIP(), not raw headers (SR-NEW-2)
// --------------------------------------------------------------------------

func TestClientKeyFromRequest_IgnoresSpoofedHeaders(t *testing.T) {
	// When TrustedProxies is empty (default in test), c.ClientIP() ignores
	// X-Forwarded-For and returns the transport-layer RemoteAddr.
	// Before the fix, clientKeyFromRequest parsed X-Forwarded-For directly,
	// letting attackers rotate rate-limiter keys via header spoofing.
	engine := ginlib.New()
	engine.TrustedPlatform = ""
	// SetTrustedProxies(nil) = trust nobody → c.ClientIP() uses RemoteAddr
	_ = engine.SetTrustedProxies(nil)

	var gotKey string
	engine.GET("/rate-test", func(c *ginlib.Context) {
		gotKey = clientKeyFromRequest(c)
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/rate-test", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	req.Header.Set("X-Real-IP", "9.10.11.12")
	req.RemoteAddr = "192.168.1.100:12345"
	engine.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	// With no trusted proxies, c.ClientIP() should return RemoteAddr IP,
	// NOT the spoofed X-Forwarded-For value.
	if gotKey == "1.2.3.4" || gotKey == "5.6.7.8" || gotKey == "9.10.11.12" {
		t.Errorf("clientKeyFromRequest = %q — used spoofed header instead of transport IP", gotKey)
	}
	if gotKey != "192.168.1.100" {
		t.Errorf("clientKeyFromRequest = %q, want %q (RemoteAddr)", gotKey, "192.168.1.100")
	}
}

func TestRunServer_MissingRoutes(t *testing.T) {
	gw := NewServer("test-no-routes", 0, true, false, nil)
	err := gw.RunServer()
	if err == nil {
		t.Error("expected error when no routes are defined")
	}
}

func TestRunServer_MissingName(t *testing.T) {
	gw := NewServer("", 0, true, false, nil)
	gw.Name = "" // Ensure blank.
	err := gw.RunServer()
	if err == nil {
		t.Error("expected error when Name is empty")
	}
}

func TestRunServer_PortTooHigh(t *testing.T) {
	gw := NewServer("test", 70000, true, false, nil)
	err := gw.RunServer()
	if err == nil {
		t.Error("expected error when port > 65535")
	}
}

// --------------------------------------------------------------------------
// SR-NEW-4: binding error must NOT leak internal details to client
// --------------------------------------------------------------------------

// TestNewRouteFunc_BindingError_NoDetailLeak verifies SR-NEW-4: when a request
// body fails JSON binding (e.g. malformed JSON or type mismatch), the HTTP
// response must contain only "Bad Request" — never struct field names,
// validation rules, type information, or internal paths.
func TestNewRouteFunc_BindingError_NoDetailLeak(t *testing.T) {
	// Create a minimal Gin wrapper and register a POST route with JSON binding.
	gw := NewServer("test-bind-leak", 0, false, false, nil)
	if gw == nil {
		t.Fatal("NewServer returned nil")
	}

	type bindTarget struct {
		SecretField string `json:"secret_field" binding:"required"`
		Amount      int    `json:"amount" binding:"required"`
	}

	gw.Routes = map[string]*RouteDefinition{
		"*": {
			Routes: []*Route{
				{
					RelativePath:    "/bind-test",
					Method:          ginhttpmethod.POST,
					Binding:         ginbindtype.BindJson,
					BindingInputPtr: &bindTarget{},
					Handler: func(c *ginlib.Context, bindingInputPtr interface{}) {
						c.String(200, "ok")
					},
				},
			},
		},
	}

	// Setup routes on the engine (calls newRouteFunc internally).
	count := gw.setupRoutes()
	if count == 0 {
		t.Fatal("setupRoutes returned 0 — route not registered")
	}

	// Send malformed JSON that will trigger a binding error.
	malformedBody := bytes.NewBufferString(`{"amount": "not-a-number"`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/bind-test", malformedBody)
	req.Header.Set("Content-Type", "application/json")
	gw.Engine().ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	body := w.Body.String()

	// Must be exactly the generic message.
	if body != "Bad Request" {
		t.Errorf("response body = %q, want %q", body, "Bad Request")
	}

	// Must NOT leak any internal details — struct field names, types, paths,
	// binding type, validation rules.
	for _, leak := range []string{
		"SecretField", "secret_field", "Amount", "amount",
		"bindTarget", "BindJson", "cannot unmarshal",
		"json:", "binding:", "required", "POST /bind-test",
		"Binding:", "Failed on",
	} {
		if bytes.Contains(w.Body.Bytes(), []byte(leak)) {
			t.Errorf("response body contains internal detail %q — binding error leaked to client", leak)
		}
	}
}
