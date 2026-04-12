package systemd

import (
	"testing"
)

// NOTE: The ServiceProgram.Launch() method calls service.New() and log.Fatalf()
// on failure, which makes it impossible to test without actual systemd/service
// infrastructure. The tests below cover struct population, field access, and
// the run() method's port validation logic.

// TestServiceProgramStructPopulation verifies that all fields of ServiceProgram
// can be set and read correctly.
func TestServiceProgramStructPopulation(t *testing.T) {
	startCalled := false
	stopCalled := false

	sp := &ServiceProgram{
		ServiceName: "test-service",
		DisplayName: "Test Service Display",
		Description: "A test service description",
		Port:        8080,
		StartServiceHandler: func(port int) {
			startCalled = true
		},
		StopServiceHandler: func() {
			stopCalled = true
		},
	}

	if sp.ServiceName != "test-service" {
		t.Errorf("ServiceName = %q, expected %q", sp.ServiceName, "test-service")
	}
	if sp.DisplayName != "Test Service Display" {
		t.Errorf("DisplayName = %q, expected %q", sp.DisplayName, "Test Service Display")
	}
	if sp.Description != "A test service description" {
		t.Errorf("Description = %q, expected %q", sp.Description, "A test service description")
	}
	if sp.Port != 8080 {
		t.Errorf("Port = %d, expected %d", sp.Port, 8080)
	}

	// Verify handlers are callable
	sp.StartServiceHandler(sp.Port)
	if !startCalled {
		t.Error("StartServiceHandler was not invoked")
	}

	sp.StopServiceHandler()
	if !stopCalled {
		t.Error("StopServiceHandler was not invoked")
	}
}

// TestServiceProgramZeroPort verifies that port 0 is accepted (some services
// may not need a port).
func TestServiceProgramZeroPort(t *testing.T) {
	handlerPort := -1

	sp := &ServiceProgram{
		ServiceName: "no-port-service",
		DisplayName: "No Port Service",
		Description: "Service without port",
		Port:        0,
		StartServiceHandler: func(port int) {
			handlerPort = port
		},
	}

	// Call run() directly - it runs synchronously in this context
	sp.run()

	if handlerPort != 0 {
		t.Errorf("StartServiceHandler received port %d, expected 0", handlerPort)
	}
}

// TestServiceProgramNilHandlers verifies that run() does not panic when
// handlers are nil.
func TestServiceProgramNilHandlers(t *testing.T) {
	sp := &ServiceProgram{
		ServiceName:         "nil-handler-service",
		DisplayName:         "Nil Handler Service",
		Description:         "Service with nil handlers",
		Port:                8080,
		StartServiceHandler: nil,
		StopServiceHandler:  nil,
	}

	// Should not panic
	sp.run()
}

// TestServiceProgramNilReceiver verifies that run() handles nil receiver
// gracefully (via the nil check + recover in run).
func TestServiceProgramNilReceiver(t *testing.T) {
	var sp *ServiceProgram

	// Should not panic due to nil check at start of run()
	sp.run()
}

// TestServiceProgramInvalidPortNegative verifies that run() skips the
// StartServiceHandler when port is negative (outside 0-65535 range).
func TestServiceProgramInvalidPortNegative(t *testing.T) {
	startCalled := false

	sp := &ServiceProgram{
		ServiceName: "neg-port",
		DisplayName: "Negative Port",
		Description: "Service with negative port",
		Port:        -1,
		StartServiceHandler: func(port int) {
			startCalled = true
		},
	}

	sp.run()

	if startCalled {
		t.Error("StartServiceHandler should NOT be called with negative port")
	}
}

// TestServiceProgramInvalidPortTooHigh verifies that run() skips the
// StartServiceHandler when port exceeds 65535.
func TestServiceProgramInvalidPortTooHigh(t *testing.T) {
	startCalled := false

	sp := &ServiceProgram{
		ServiceName: "high-port",
		DisplayName: "High Port",
		Description: "Service with port > 65535",
		Port:        70000,
		StartServiceHandler: func(port int) {
			startCalled = true
		},
	}

	sp.run()

	if startCalled {
		t.Error("StartServiceHandler should NOT be called with port > 65535")
	}
}

// TestServiceProgramValidPortBoundary verifies that run() invokes
// StartServiceHandler for boundary valid ports (0 and 65535).
func TestServiceProgramValidPortBoundary(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"port_0", 0},
		{"port_65535", 65535},
		{"port_1", 1},
		{"port_443", 443},
		{"port_8080", 8080},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			receivedPort := -1

			sp := &ServiceProgram{
				ServiceName: "boundary-test",
				Port:        tc.port,
				StartServiceHandler: func(port int) {
					called = true
					receivedPort = port
				},
			}

			sp.run()

			if !called {
				t.Errorf("StartServiceHandler should be called for port %d", tc.port)
			}
			if receivedPort != tc.port {
				t.Errorf("StartServiceHandler received port %d, expected %d", receivedPort, tc.port)
			}
		})
	}
}

// TestStopMethodNilReceiver verifies that Stop handles nil receiver
// via the nil check.
func TestStopMethodNilReceiver(t *testing.T) {
	var sp *ServiceProgram

	// Stop checks for nil receiver before invoking handler
	err := sp.Stop(nil)
	if err != nil {
		t.Errorf("Stop() on nil receiver returned error: %v", err)
	}
}

// TestStopMethodInvokesHandler verifies that Stop calls StopServiceHandler.
func TestStopMethodInvokesHandler(t *testing.T) {
	stopCalled := false

	sp := &ServiceProgram{
		StopServiceHandler: func() {
			stopCalled = true
		},
	}

	err := sp.Stop(nil)
	if err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
	if !stopCalled {
		t.Error("StopServiceHandler was not invoked by Stop()")
	}
}

// TestStopMethodNilHandler verifies that Stop does not panic when
// StopServiceHandler is nil.
func TestStopMethodNilHandler(t *testing.T) {
	sp := &ServiceProgram{
		StopServiceHandler: nil,
	}

	err := sp.Stop(nil)
	if err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}

// Coverage gap notes:
// - Launch() cannot be tested without systemd/service infrastructure. It calls
//   service.New() which requires OS-level service management, and log.Fatalf()
//   on error which terminates the process.
// - Start() delegates to run() via goroutine, which we test synchronously above.
