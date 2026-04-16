package gin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/cors"
	ginlib "github.com/gin-gonic/gin"
)

// TG-2: CORS fail-closed tests — verify SEC-003 fix.
// When AllowAllOrigins=false and no AllowOrigins are configured,
// cross-origin requests must be rejected (no Access-Control-Allow-Origin header).

func TestCORS_FailClosed_EmptyAllowOrigins(t *testing.T) {
	engine := ginlib.New()
	g := &Gin{_ginEngine: engine}

	group := engine.Group("/api")

	// SEC-003 scenario: AllowAllOrigins=false, empty AllowOrigins, nil AllowOriginFunc
	corsConfig := &cors.Config{
		AllowAllOrigins: false,
		AllowOrigins:    []string{},
		AllowMethods:    []string{"GET", "POST"},
	}
	g.setupCorsMiddleware(group, corsConfig)

	group.GET("/test", func(c *ginlib.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Preflight request from an arbitrary origin
	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// CORS should NOT allow this origin
	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header, got %q", origin)
	}
}

func TestCORS_FailClosed_NonHTTPOriginsFiltered(t *testing.T) {
	engine := ginlib.New()
	g := &Gin{_ginEngine: engine}

	group := engine.Group("/api")

	// AllowOrigins has entries but none start with "http" — they get
	// filtered by setupCorsMiddleware (lines 774-785), leaving zero
	// valid origins. The inner gate (SEC-003) should still fail closed.
	corsConfig := &cors.Config{
		AllowAllOrigins: false,
		AllowOrigins:    []string{"ftp://badhost.com", "tcp://wrong"},
		AllowMethods:    []string{"GET"},
	}
	g.setupCorsMiddleware(group, corsConfig)

	group.GET("/test", func(c *ginlib.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("expected no Access-Control-Allow-Origin header for filtered origins, got %q", origin)
	}
}

func TestCORS_AllowedOrigin_Positive(t *testing.T) {
	engine := ginlib.New()
	g := &Gin{_ginEngine: engine}

	group := engine.Group("/api")

	// Positive control: a valid origin IS configured
	corsConfig := &cors.Config{
		AllowAllOrigins: false,
		AllowOrigins:    []string{"http://trusted.example.com"},
		AllowMethods:    []string{"GET", "POST"},
	}
	g.setupCorsMiddleware(group, corsConfig)

	group.GET("/test", func(c *ginlib.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Simple cross-origin GET (not preflight) from the trusted origin
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "http://trusted.example.com")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://trusted.example.com" {
		t.Errorf("expected Access-Control-Allow-Origin=%q, got %q", "http://trusted.example.com", origin)
	}
}

func TestCORS_UntrustedOrigin_Rejected(t *testing.T) {
	engine := ginlib.New()
	g := &Gin{_ginEngine: engine}

	group := engine.Group("/api")

	corsConfig := &cors.Config{
		AllowAllOrigins: false,
		AllowOrigins:    []string{"http://trusted.example.com"},
		AllowMethods:    []string{"GET"},
	}
	g.setupCorsMiddleware(group, corsConfig)

	group.GET("/test", func(c *ginlib.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Request from an UNTRUSTED origin
	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("expected no Access-Control-Allow-Origin for untrusted origin, got %q", origin)
	}
}
