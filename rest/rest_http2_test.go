package rest

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSharedTransport_HTTP2HealthCheckConfigured verifies the shared Transport carries
// the HTTP/2 ping health check. Without it, a pooled HTTP/2 connection whose peer
// vanished silently is reused until the kernel's TCP retransmission gives up.
func TestSharedTransport_HTTP2HealthCheckConfigured(t *testing.T) {
	mu.Lock()
	sharedTransport = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		sharedTransport = nil
		mu.Unlock()
	}()

	tr := getSharedTransport()
	if tr.HTTP2 == nil {
		t.Fatal("shared Transport has no HTTP2 config — dead HTTP/2 connections are never detected")
	}
	if tr.HTTP2.SendPingTimeout <= 0 {
		t.Errorf("HTTP2.SendPingTimeout = %v, expected > 0 (0 disables the health check)", tr.HTTP2.SendPingTimeout)
	}
	if tr.HTTP2.PingTimeout <= 0 {
		t.Errorf("HTTP2.PingTimeout = %v, expected > 0", tr.HTTP2.PingTimeout)
	}
}

// freezingProxy is a TCP proxy whose existing connections can be frozen: once frozen,
// it keeps reading from both sides but forwards nothing and never closes. To the
// client's kernel the connection stays healthy (bytes are ACKed), which is how a
// replaced load-balancer node looks — no FIN, no RST, just silence. Connections
// accepted after the freeze forward normally, standing in for the new node.
type freezingProxy struct {
	ln     net.Listener
	target string

	mu     sync.Mutex
	frozen []*atomic.Bool
	conns  []net.Conn
}

func newFreezingProxy(t *testing.T, target string) *freezingProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}
	p := &freezingProxy{ln: ln, target: target}
	go p.serve()
	return p
}

func (p *freezingProxy) serve() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		u, err := net.Dial("tcp", p.target)
		if err != nil {
			_ = c.Close()
			continue
		}
		f := &atomic.Bool{}
		p.mu.Lock()
		p.frozen = append(p.frozen, f)
		p.conns = append(p.conns, c, u)
		p.mu.Unlock()
		go pipeUnlessFrozen(u, c, f)
		go pipeUnlessFrozen(c, u, f)
	}
}

func pipeUnlessFrozen(dst io.Writer, src io.Reader, frozen *atomic.Bool) {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 && !frozen.Load() {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// freezeExisting silences every connection accepted so far.
func (p *freezingProxy) freezeExisting() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, f := range p.frozen {
		f.Store(true)
	}
}

func (p *freezingProxy) close() {
	_ = p.ln.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
}

// TestPOST_HTTP2RecoversFromSilentlyDeadConnection reproduces the 2026-10-08 incident:
// an HTTP/2 connection to a load-balancer node goes silent, and every later POST was
// queued onto it until TCP gave up ~17 minutes later. With the ping health check the
// Transport drops the dead connection and a later POST succeeds on a fresh one.
func TestPOST_HTTP2RecoversFromSilentlyDeadConnection(t *testing.T) {
	var sawHTTP2 atomic.Bool
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 {
			sawHTTP2.Store(true)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	proxy := newFreezingProxy(t, server.Listener.Addr().String())
	defer proxy.close()
	url := "https://" + proxy.ln.Addr().String() + "/reporthealth"

	mu.Lock()
	origSendPing, origPing, origTimeout := http2SendPingTimeout, http2PingTimeout, clientTimeoutSeconds
	http2SendPingTimeout, http2PingTimeout = 300*time.Millisecond, 300*time.Millisecond
	clientTimeoutSeconds = 1
	sharedTransport = nil
	mu.Unlock()
	defer func() {
		mu.Lock()
		http2SendPingTimeout, http2PingTimeout, clientTimeoutSeconds = origSendPing, origPing, origTimeout
		sharedTransport = nil
		mu.Unlock()
	}()

	// Trust the test server's certificate WITHOUT setting clientTlsConfig: production
	// negotiates HTTP/2 only when no CA is configured, so the test must not take the
	// HTTP/1.1 path. These fields are read on the Transport's first round trip.
	tr := getSharedTransport()
	tr.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tr.TLSClientConfig.InsecureSkipVerify = false
	tr.ForceAttemptHTTP2 = true

	if status, _, err := POST(url, nil, "{}"); err != nil || status != http.StatusOK {
		t.Fatalf("initial POST: status=%d err=%v", status, err)
	}
	if !sawHTTP2.Load() {
		t.Fatal("initial POST did not use HTTP/2 — the test would not exercise the bug")
	}

	proxy.freezeExisting()

	// The first POST after the freeze may fail: it can land on the dead connection
	// before the ping has fired. What matters is that the client recovers instead
	// of reusing the dead connection forever.
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		status, _, err := POST(url, nil, "{}")
		if err == nil && status == http.StatusOK {
			return
		}
		lastErr = err
	}
	t.Fatalf("POST never recovered from a silently dead HTTP/2 connection within 10s (last error: %v)", lastErr)
}
