package aws

/*
 * Copyright 2020-2025 Aldelo, LP
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
	"testing"
	"time"
)

// TestNilReceiverSetters tests that all setter methods handle nil receiver gracefully
func TestNilReceiverSetters(t *testing.T) {
	var h2 *AwsHttp2Client

	// Test all setter methods - they should not panic when called on nil receiver
	t.Run("SetDialContextKeepAlive", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetDialContextKeepAlive panicked on nil receiver: %v", r)
			}
		}()
		h2.SetDialContextKeepAlive(10 * time.Second)
	})

	t.Run("SetDialContextConnectTimeout", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetDialContextConnectTimeout panicked on nil receiver: %v", r)
			}
		}()
		h2.SetDialContextConnectTimeout(10 * time.Second)
	})

	t.Run("SetExpectContinueTimeout", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetExpectContinueTimeout panicked on nil receiver: %v", r)
			}
		}()
		h2.SetExpectContinueTimeout(1 * time.Second)
	})

	t.Run("SetOverallTimeout", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetOverallTimeout panicked on nil receiver: %v", r)
			}
		}()
		h2.SetOverallTimeout(60 * time.Second)
	})

	t.Run("SetMaxIdleConns", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetMaxIdleConns panicked on nil receiver: %v", r)
			}
		}()
		h2.SetMaxIdleConns(100)
	})

	t.Run("SetMaxIdleConnsPerHost", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetMaxIdleConnsPerHost panicked on nil receiver: %v", r)
			}
		}()
		h2.SetMaxIdleConnsPerHost(500)
	})

	t.Run("SetMaxConnsPerHost", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetMaxConnsPerHost panicked on nil receiver: %v", r)
			}
		}()
		h2.SetMaxConnsPerHost(100)
	})

	t.Run("SetIdleConnTimeout", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetIdleConnTimeout panicked on nil receiver: %v", r)
			}
		}()
		h2.SetIdleConnTimeout(300 * time.Second)
	})

	t.Run("SetDisableKeepAlives", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetDisableKeepAlives panicked on nil receiver: %v", r)
			}
		}()
		h2.SetDisableKeepAlives(false)
	})

	t.Run("SetDisableCompressions", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetDisableCompressions panicked on nil receiver: %v", r)
			}
		}()
		h2.SetDisableCompressions(false)
	})

	t.Run("SetTlsHandshakeTimeout", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetTlsHandshakeTimeout panicked on nil receiver: %v", r)
			}
		}()
		h2.SetTlsHandshakeTimeout(10 * time.Second)
	})

	t.Run("SetResponseHeaderTimeout", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SetResponseHeaderTimeout panicked on nil receiver: %v", r)
			}
		}()
		h2.SetResponseHeaderTimeout(30 * time.Second)
	})
}

// TestNilReceiverNewHttp2Client tests that NewHttp2Client handles nil receiver gracefully
func TestNilReceiverNewHttp2Client(t *testing.T) {
	var h2 *AwsHttp2Client

	client, err := h2.NewHttp2Client()

	if client != nil {
		t.Error("Expected nil client when calling NewHttp2Client on nil receiver")
	}

	if err == nil {
		t.Error("Expected error when calling NewHttp2Client on nil receiver")
	}

	expectedError := "AwsHttp2Client receiver is nil"
	if err != nil && err.Error() != expectedError {
		t.Errorf("Expected error message '%s', got '%s'", expectedError, err.Error())
	}
}

// TestSettersWithValidReceiver tests that setters still work correctly with valid receiver
func TestSettersWithValidReceiver(t *testing.T) {
	h2 := &AwsHttp2Client{}

	// Test that setters work correctly
	h2.SetDialContextKeepAlive(15 * time.Second)
	if h2.Options == nil {
		t.Error("Options should be initialized")
	}
	if h2.Options.DialContextKeepAlive == nil {
		t.Error("DialContextKeepAlive should be set")
	}
	if *h2.Options.DialContextKeepAlive != 15*time.Second {
		t.Errorf("Expected 15s, got %v", *h2.Options.DialContextKeepAlive)
	}

	h2.SetMaxIdleConns(200)
	if h2.Options.MaxIdleConns == nil {
		t.Error("MaxIdleConns should be set")
	}
	if *h2.Options.MaxIdleConns != 200 {
		t.Errorf("Expected 200, got %v", *h2.Options.MaxIdleConns)
	}

	h2.SetDisableKeepAlives(true)
	if h2.Options.DisableKeepAlive == nil {
		t.Error("DisableKeepAlive should be set")
	}
	if *h2.Options.DisableKeepAlive != true {
		t.Errorf("Expected true, got %v", *h2.Options.DisableKeepAlive)
	}
}

// TestNewHttp2ClientWithValidReceiver tests that NewHttp2Client works correctly with valid receiver
func TestNewHttp2ClientWithValidReceiver(t *testing.T) {
	h2 := &AwsHttp2Client{}

	client, err := h2.NewHttp2Client()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if client == nil {
		t.Error("Expected non-nil client")
	}

	// Verify defaults were set
	if client.Timeout == 0 {
		t.Error("Client timeout should be set to default value")
	}
}
