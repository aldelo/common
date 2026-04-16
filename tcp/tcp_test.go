package tcp

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// getFreePort asks the OS to assign a free TCP port and returns it.
func getFreePort(t *testing.T) uint {
	t.Helper()
	ln, err := net.Listen("tcp4", ":0")
	if err != nil {
		t.Fatalf("getFreePort: %v", err)
	}
	port := uint(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()
	return port
}

type serverOpts struct {
	acceptHandler        func(clientIP string)
	listenerErrorHandler func(err error)
	clientReceiveHandler func(clientIP string, data []byte, writeToClientFunc func(writeData []byte, clientIP string) error)
	clientErrorHandler   func(clientIP string, err error)
}

// safeCloseServer shuts down a TCPServer and waits for the listener goroutine
// to exit. The production Close() method is now race-free (the listener goroutine
// holds a local copy of the net.Listener obtained under _mux, and Close() calls
// listener.Close() to unblock Accept()). This helper adds the synchronization
// wait via listenerDone channel that tests need to avoid teardown races.
func safeCloseServer(srv *TCPServer, listenerDone <-chan struct{}) {
	if srv == nil {
		return
	}

	// Read listener ref under lock — this is safe because the goroutine
	// only READS _tcpListener (no write race from our side).
	srv._mux.Lock()
	listener := srv._tcpListener
	srv._mux.Unlock()

	// Close the underlying listener to make Accept() return an error.
	if listener != nil {
		_ = listener.Close()
	}

	// Wait for the goroutine to exit (it fires ListenerErrorHandler then returns).
	if listenerDone != nil {
		select {
		case <-listenerDone:
		case <-time.After(5 * time.Second):
		}
	}

	// Now the goroutine has exited. Clean up server state.
	srv._mux.Lock()
	defer srv._mux.Unlock()

	if srv._clientEnd != nil {
		for _, ch := range srv._clientEnd {
			if ch != nil {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}
	if srv._clients != nil {
		for _, v := range srv._clients {
			if v != nil {
				_ = v.Close()
			}
		}
	}
	srv._clients = nil
	srv._clientEnd = nil
	srv._tcpListener = nil
	srv._serving = false
}

// startTestServer creates a TCPServer on an OS-assigned port, wires the
// handlers, calls Serve(), and registers a cleanup via t.Cleanup.
func startTestServer(t *testing.T, opts serverOpts) (*TCPServer, uint, <-chan struct{}) {
	t.Helper()
	port := getFreePort(t)

	acceptCh := make(chan struct{}, 16)
	listenerDoneCh := make(chan struct{}, 1)

	wrappedAccept := func(clientIP string) {
		select {
		case acceptCh <- struct{}{}:
		default:
		}
		if opts.acceptHandler != nil {
			opts.acceptHandler(clientIP)
		}
	}

	wrappedListenerError := func(err error) {
		if opts.listenerErrorHandler != nil {
			opts.listenerErrorHandler(err)
		}
		select {
		case listenerDoneCh <- struct{}{}:
		default:
		}
	}

	srv := &TCPServer{
		Port:                  port,
		ListenerAcceptHandler: wrappedAccept,
		ListenerErrorHandler:  wrappedListenerError,
		ClientReceiveHandler:  opts.clientReceiveHandler,
		ClientErrorHandler:    opts.clientErrorHandler,
		ReadDeadlineDuration:  300 * time.Millisecond,
		ReaderYieldDuration:   5 * time.Millisecond,
	}

	// Register cleanup FIRST (LIFO: runs last, after client cleanups).
	t.Cleanup(func() { safeCloseServer(srv, listenerDoneCh) })

	if err := srv.Serve(); err != nil {
		t.Fatalf("Serve failed: %v", err)
	}

	return srv, port, acceptCh
}

// waitForAccept blocks until the server's accept handler fires or timeout.
func waitForAccept(t *testing.T, acceptCh <-chan struct{}) {
	t.Helper()
	select {
	case <-acceptCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to accept client")
	}
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

func TestServerClientRoundTrip(t *testing.T) {
	serverGotData := make(chan []byte, 1)
	clientGotData := make(chan []byte, 1)

	_, port, acceptCh := startTestServer(t, serverOpts{
		clientReceiveHandler: func(clientIP string, data []byte, writeBack func([]byte, string) error) {
			serverGotData <- append([]byte(nil), data...)
			_ = writeBack([]byte("echo:"+string(data)), clientIP)
		},
		clientErrorHandler: func(clientIP string, err error) {},
	})

	client := &TCPClient{
		ServerIP:             "127.0.0.1",
		ServerPort:           port,
		ReadDeadLineDuration: 2 * time.Second,
		ReadBufferSize:       1024,
		ReaderYieldDuration:  5 * time.Millisecond,
		ReceiveHandler: func(data []byte) {
			clientGotData <- append([]byte(nil), data...)
		},
		ErrorHandler: func(err error, socketCloseFunc func()) {},
	}

	if err := client.Dial(); err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	waitForAccept(t, acceptCh)

	if err := client.StartReader(); err != nil {
		t.Fatalf("StartReader failed: %v", err)
	}

	msg := []byte("hello-tcp")
	if err := client.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case got := <-serverGotData:
		if string(got) != "hello-tcp" {
			t.Errorf("server received %q, want %q", got, "hello-tcp")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to receive data")
	}

	select {
	case got := <-clientGotData:
		if string(got) != "echo:hello-tcp" {
			t.Errorf("client received %q, want %q", got, "echo:hello-tcp")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for client to receive echo")
	}
}

func TestClientDialNonExistentServer(t *testing.T) {
	port := getFreePort(t)

	client := &TCPClient{
		ServerIP:       "127.0.0.1",
		ServerPort:     port,
		ReceiveHandler: func(data []byte) {},
		ErrorHandler:   func(err error, f func()) {},
	}

	err := client.Dial()
	if err == nil {
		client.Close()
		t.Fatal("expected Dial to fail when no server is listening, got nil error")
	}
}

func TestClientDialMissingHandlers(t *testing.T) {
	c := &TCPClient{ServerIP: "127.0.0.1", ServerPort: 9999}
	if err := c.Dial(); err == nil {
		t.Error("expected error when ReceiveHandler is nil")
	}

	c2 := &TCPClient{
		ServerIP:       "127.0.0.1",
		ServerPort:     9999,
		ReceiveHandler: func([]byte) {},
	}
	if err := c2.Dial(); err == nil {
		t.Error("expected error when ErrorHandler is nil")
	}
}

func TestClientWriteBeforeDial(t *testing.T) {
	c := &TCPClient{
		ServerIP:       "127.0.0.1",
		ServerPort:     9999,
		ReceiveHandler: func([]byte) {},
		ErrorHandler:   func(error, func()) {},
	}
	err := c.Write([]byte("data"))
	if err == nil {
		t.Error("expected error when writing before Dial, got nil")
	}
}

func TestClientWriteEmptyData(t *testing.T) {
	_, port, acceptCh := startTestServer(t, serverOpts{
		clientReceiveHandler: func(string, []byte, func([]byte, string) error) {},
		clientErrorHandler:   func(string, error) {},
	})

	client := &TCPClient{
		ServerIP:       "127.0.0.1",
		ServerPort:     port,
		ReceiveHandler: func([]byte) {},
		ErrorHandler:   func(error, func()) {},
	}
	if err := client.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	waitForAccept(t, acceptCh)

	err := client.Write([]byte{})
	if err == nil {
		t.Error("expected error when writing empty data")
	}
}

func TestServerCloseAndCleanup(t *testing.T) {
	closedCh := make(chan struct{}, 1)
	listenerDoneCh := make(chan struct{}, 1)

	port := getFreePort(t)
	acceptCh := make(chan struct{}, 16)

	srv := &TCPServer{
		Port: port,
		ListenerAcceptHandler: func(clientIP string) {
			select {
			case acceptCh <- struct{}{}:
			default:
			}
		},
		ListenerErrorHandler: func(err error) {
			select {
			case closedCh <- struct{}{}:
			default:
			}
			select {
			case listenerDoneCh <- struct{}{}:
			default:
			}
		},
		ClientReceiveHandler: func(string, []byte, func([]byte, string) error) {},
		ClientErrorHandler:   func(string, error) {},
		ReadDeadlineDuration: 300 * time.Millisecond,
		ReaderYieldDuration:  5 * time.Millisecond,
	}

	if err := srv.Serve(); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	client := &TCPClient{
		ServerIP:             "127.0.0.1",
		ServerPort:           port,
		ReadDeadLineDuration: 300 * time.Millisecond,
		ReceiveHandler:       func([]byte) {},
		ErrorHandler:         func(error, func()) {},
	}
	if err := client.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	select {
	case <-acceptCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for accept")
	}

	client.Close()
	safeCloseServer(srv, listenerDoneCh)

	select {
	case <-closedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for listener error handler after Close")
	}

	clients := srv.GetConnectedClients()
	if len(clients) != 0 {
		t.Errorf("expected 0 connected clients after Close, got %d", len(clients))
	}
}

func TestServerServeInvalidPort(t *testing.T) {
	srv := &TCPServer{Port: 0}
	err := srv.Serve()
	if err == nil {
		t.Error("expected error for Port=0")
	}
}

func TestServerDoubleServe(t *testing.T) {
	srv, _, _ := startTestServer(t, serverOpts{
		clientReceiveHandler: func(string, []byte, func([]byte, string) error) {},
		clientErrorHandler:   func(string, error) {},
	})

	err := srv.Serve()
	if err == nil {
		t.Error("expected error when calling Serve on an already-serving server")
	}
}

func TestMultipleClientsConnectAndReceive(t *testing.T) {
	var mu sync.Mutex
	receivedFrom := make(map[string][]byte)
	allDone := make(chan struct{}, 1)
	const numClients = 3

	_, port, acceptCh := startTestServer(t, serverOpts{
		clientReceiveHandler: func(clientIP string, data []byte, writeBack func([]byte, string) error) {
			mu.Lock()
			receivedFrom[clientIP] = append([]byte(nil), data...)
			count := len(receivedFrom)
			mu.Unlock()
			if count == numClients {
				select {
				case allDone <- struct{}{}:
				default:
				}
			}
		},
		clientErrorHandler: func(string, error) {},
	})

	for i := 0; i < numClients; i++ {
		c := &TCPClient{
			ServerIP:       "127.0.0.1",
			ServerPort:     port,
			ReceiveHandler: func([]byte) {},
			ErrorHandler:   func(error, func()) {},
		}
		if err := c.Dial(); err != nil {
			t.Fatalf("client %d Dial: %v", i, err)
		}
		t.Cleanup(func() { c.Close() })
		waitForAccept(t, acceptCh)

		msg := fmt.Sprintf("client-%d", i)
		if err := c.Write([]byte(msg)); err != nil {
			t.Fatalf("client %d Write: %v", i, err)
		}
	}

	select {
	case <-allDone:
	case <-time.After(5 * time.Second):
		mu.Lock()
		count := len(receivedFrom)
		mu.Unlock()
		t.Fatalf("timed out: only got %d/%d client messages", count, numClients)
	}
}

func TestClientStopReaderIdempotent(t *testing.T) {
	_, port, acceptCh := startTestServer(t, serverOpts{
		clientReceiveHandler: func(string, []byte, func([]byte, string) error) {},
		clientErrorHandler:   func(string, error) {},
	})

	client := &TCPClient{
		ServerIP:             "127.0.0.1",
		ServerPort:           port,
		ReadDeadLineDuration: 300 * time.Millisecond,
		ReaderYieldDuration:  5 * time.Millisecond,
		ReceiveHandler:       func([]byte) {},
		ErrorHandler:         func(error, func()) {},
	}
	if err := client.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	waitForAccept(t, acceptCh)

	if err := client.StartReader(); err != nil {
		t.Fatalf("StartReader: %v", err)
	}

	client.StopReader()
	client.StopReader()
}

func TestResolveTcpAddrErrors(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		port uint
	}{
		{"empty ip", "", 8080},
		{"zero port", "127.0.0.1", 0},
		{"port too high", "127.0.0.1", 70000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &TCPClient{ServerIP: tc.ip, ServerPort: tc.port}
			_, err := c.resolveTcpAddr()
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestNilReceiverSafety(t *testing.T) {
	var c *TCPClient
	c.Close()
	c.StopReader()

	if err := c.Dial(); err == nil {
		t.Error("expected error on nil client Dial")
	}
	if err := c.Write([]byte("x")); err == nil {
		t.Error("expected error on nil client Write")
	}
	if _, _, err := c.Read(); err == nil {
		t.Error("expected error on nil client Read")
	}
	if err := c.StartReader(); err == nil {
		t.Error("expected error on nil client StartReader")
	}

	var s *TCPServer
	s.Close()
	if err := s.Serve(); err == nil {
		t.Error("expected error on nil server Serve")
	}
	clients := s.GetConnectedClients()
	if len(clients) != 0 {
		t.Errorf("expected empty client list from nil server, got %d", len(clients))
	}
}

// TestServerSurvivesClientHandlerPanic verifies that a panic inside
// ClientReceiveHandler is recovered and does not crash the server's
// listener loop — subsequent clients can still connect and communicate.
// Covers COMMON-R3-001 panic recovery in the per-client goroutine.
func TestServerSurvivesClientHandlerPanic(t *testing.T) {
	var callCount int32
	serverGotData := make(chan []byte, 1)

	_, port, acceptCh := startTestServer(t, serverOpts{
		clientReceiveHandler: func(clientIP string, data []byte, writeBack func([]byte, string) error) {
			n := atomicAddInt32(&callCount, 1)
			if n == 1 {
				panic("test panic in client handler (TG-3)")
			}
			serverGotData <- append([]byte(nil), data...)
		},
		clientErrorHandler: func(string, error) {},
	})

	// Client 1: triggers the panic in the handler goroutine.
	conn1, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("client 1 dial: %v", err)
	}
	waitForAccept(t, acceptCh)
	_, _ = conn1.Write([]byte("trigger-panic"))
	time.Sleep(300 * time.Millisecond) // allow panic + recovery
	_ = conn1.Close()

	// Client 2: proves the listener loop survived the panic.
	conn2, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("client 2 dial failed — server did not survive client handler panic: %v", err)
	}
	defer conn2.Close()
	waitForAccept(t, acceptCh)
	_, _ = conn2.Write([]byte("after-panic"))

	select {
	case got := <-serverGotData:
		if string(got) != "after-panic" {
			t.Errorf("server received %q, want %q", got, "after-panic")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not process client 2 data — panic recovery failed")
	}
}

// atomicAddInt32 wraps sync/atomic.AddInt32 for readability in test closures.
func atomicAddInt32(addr *int32, delta int32) int32 {
	return atomic.AddInt32(addr, delta)
}

// TestTCPServer_ConcurrentClose_NoRace verifies that calling Close() while
// the listener goroutine is blocked in Accept() does not trigger a data race.
// Before the fix (A1-F3), the listener goroutine read s._tcpListener without
// holding _mux, racing with Close() which wrote s._tcpListener = nil under
// _mux. The fix captures the listener into a goroutine-local variable under
// the lock, so Close() only needs to call listener.Close() to unblock Accept().
//
// This test MUST be run with -race to be meaningful.
func TestTCPServer_ConcurrentClose_NoRace(t *testing.T) {
	port := getFreePort(t)

	listenerDone := make(chan struct{}, 1)

	srv := &TCPServer{
		Port: port,
		ListenerErrorHandler: func(err error) {
			select {
			case listenerDone <- struct{}{}:
			default:
			}
		},
		ClientReceiveHandler: func(string, []byte, func([]byte, string) error) {},
		ClientErrorHandler:   func(string, error) {},
		ReadDeadlineDuration: 300 * time.Millisecond,
		ReaderYieldDuration:  5 * time.Millisecond,
	}

	if err := srv.Serve(); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	// Give the listener goroutine time to enter Accept().
	time.Sleep(50 * time.Millisecond)

	// Close from the main goroutine while Accept() is blocking in the
	// listener goroutine. Under -race, the old code would report a data
	// race on _tcpListener here. The fix eliminates the race.
	srv.Close()

	// Wait for the listener goroutine to exit cleanly.
	select {
	case <-listenerDone:
		// success — listener exited cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for listener goroutine to exit after Close()")
	}

	// Verify server state is cleaned up (read _serving under the lock to
	// avoid a race with the listener goroutine's cleanup path).
	srv._mux.RLock()
	serving := srv._serving
	srv._mux.RUnlock()
	if serving {
		t.Error("expected _serving to be false after Close()")
	}
	clients := srv.GetConnectedClients()
	if len(clients) != 0 {
		t.Errorf("expected 0 connected clients after Close, got %d", len(clients))
	}
}
