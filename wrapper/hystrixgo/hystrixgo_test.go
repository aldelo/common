package hystrixgo

/*
 * Copyright 2020-2026 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import (
	"context"
	"strings"
	"testing"
)

// TestNilReceiverChecks validates that all public methods properly handle nil receivers
func TestNilReceiverChecks(t *testing.T) {
	var nilCB *CircuitBreaker

	// Test Init
	err := nilCB.Init()
	if err == nil {
		t.Error("Init should return error for nil receiver")
	}
	if !strings.Contains(err.Error(), "CircuitBreaker receiver is nil") {
		t.Errorf("Init error message incorrect: %v", err)
	}

	// Test FlushAll (should not panic)
	nilCB.FlushAll()

	// Test UpdateConfig (should not panic)
	nilCB.UpdateConfig()

	// Test UpdateLogger (should not panic)
	nilCB.UpdateLogger()

	// Test Go
	_, err = nilCB.Go(nil, nil, nil)
	if err == nil {
		t.Error("Go should return error for nil receiver")
	}
	if !strings.Contains(err.Error(), "CircuitBreaker receiver is nil") {
		t.Errorf("Go error message incorrect: %v", err)
	}

	// Test GoC
	ctx := context.Background()
	_, err = nilCB.GoC(ctx, nil, nil, nil)
	if err == nil {
		t.Error("GoC should return error for nil receiver")
	}
	if !strings.Contains(err.Error(), "CircuitBreaker receiver is nil") {
		t.Errorf("GoC error message incorrect: %v", err)
	}

	// Test Do
	_, err = nilCB.Do(nil, nil, nil)
	if err == nil {
		t.Error("Do should return error for nil receiver")
	}
	if !strings.Contains(err.Error(), "CircuitBreaker receiver is nil") {
		t.Errorf("Do error message incorrect: %v", err)
	}

	// Test DoC
	_, err = nilCB.DoC(ctx, nil, nil, nil)
	if err == nil {
		t.Error("DoC should return error for nil receiver")
	}
	if !strings.Contains(err.Error(), "CircuitBreaker receiver is nil") {
		t.Errorf("DoC error message incorrect: %v", err)
	}

	// Test StartStreamHttpServer
	err = nilCB.StartStreamHttpServer()
	if err == nil {
		t.Error("StartStreamHttpServer should return error for nil receiver")
	}
	if !strings.Contains(err.Error(), "CircuitBreaker receiver is nil") {
		t.Errorf("StartStreamHttpServer error message incorrect: %v", err)
	}

	// Test StopStreamHttpServer (should not panic)
	nilCB.StopStreamHttpServer()

	// Test StartStatsdCollector
	err = nilCB.StartStatsdCollector("test", "127.0.0.1")
	if err == nil {
		t.Error("StartStatsdCollector should return error for nil receiver")
	}
	if !strings.Contains(err.Error(), "CircuitBreaker receiver is nil") {
		t.Errorf("StartStatsdCollector error message incorrect: %v", err)
	}
}

// TestHttpServerLifecycle validates the HTTP server lifecycle management
func TestHttpServerLifecycle(t *testing.T) {
	cb := &CircuitBreaker{
		CommandName: "test-server",
	}

	// Initialize the circuit breaker
	err := cb.Init()
	if err != nil {
		t.Fatalf("Failed to initialize circuit breaker: %v", err)
	}

	// Start the HTTP server
	err = cb.StartStreamHttpServer(8182)
	if err != nil {
		t.Fatalf("Failed to start stream HTTP server: %v", err)
	}

	// Verify the server reference is stored
	if cb.httpServer == nil {
		t.Error("httpServer should be set after StartStreamHttpServer")
	}

	if cb.streamHandler == nil {
		t.Error("streamHandler should be set after StartStreamHttpServer")
	}

	// Give the server a moment to start listening
	// Note: We don't test actual HTTP connectivity here as that would require
	// additional complexity and could be flaky. The important part is that
	// the server is properly initialized and can be shut down gracefully.

	// Stop the server
	cb.StopStreamHttpServer()

	// Verify cleanup
	if cb.httpServer != nil {
		t.Error("httpServer should be nil after StopStreamHttpServer")
	}

	if cb.streamHandler != nil {
		t.Error("streamHandler should be nil after StopStreamHttpServer")
	}
}
