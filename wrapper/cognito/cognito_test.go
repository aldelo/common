package cognito

import (
	"testing"

	"github.com/aldelo/common/wrapper/xray"
)

// TestNilReceiverConnect tests that calling Connect on a nil receiver returns an error
func TestNilReceiverConnect(t *testing.T) {
	var c *Cognito
	err := c.Connect()
	if err == nil {
		t.Error("Expected error when calling Connect on nil receiver, got nil")
	}
	if err.Error() != "Cognito receiver is nil" {
		t.Errorf("Expected 'Cognito receiver is nil', got '%s'", err.Error())
	}
}

// TestNilReceiverConnectWithParentSegment tests that calling Connect with parent segment on a nil receiver returns an error
func TestNilReceiverConnectWithParentSegment(t *testing.T) {
	var c *Cognito
	var seg *xray.XRayParentSegment
	err := c.Connect(seg)
	if err == nil {
		t.Error("Expected error when calling Connect on nil receiver, got nil")
	}
	if err.Error() != "Cognito receiver is nil" {
		t.Errorf("Expected 'Cognito receiver is nil', got '%s'", err.Error())
	}
}

// TestNilReceiverDisconnect tests that calling Disconnect on a nil receiver doesn't panic
func TestNilReceiverDisconnect(t *testing.T) {
	var c *Cognito
	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Disconnect panicked on nil receiver: %v", r)
		}
	}()
	c.Disconnect()
}

// TestNilReceiverUpdateParentSegment tests that calling UpdateParentSegment on a nil receiver doesn't panic
func TestNilReceiverUpdateParentSegment(t *testing.T) {
	var c *Cognito
	var seg *xray.XRayParentSegment
	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("UpdateParentSegment panicked on nil receiver: %v", r)
		}
	}()
	c.UpdateParentSegment(seg)
}

// TestNilReceiverGetOpenIdTokenForDeveloperIdentity tests that calling getOpenIdTokenForDeveloperIdentity on a nil receiver returns an error
func TestNilReceiverGetOpenIdTokenForDeveloperIdentity(t *testing.T) {
	var c *Cognito
	identityId, token, err := c.getOpenIdTokenForDeveloperIdentity("pool-id", "provider-id", "provider-name", nil)
	if err == nil {
		t.Error("Expected error when calling getOpenIdTokenForDeveloperIdentity on nil receiver, got nil")
	}
	if err.Error() != "Cognito receiver is nil" {
		t.Errorf("Expected 'Cognito receiver is nil', got '%s'", err.Error())
	}
	if identityId != "" {
		t.Errorf("Expected empty identityId, got %s", identityId)
	}
	if token != "" {
		t.Errorf("Expected empty token, got %s", token)
	}
}

// TestNilReceiverNewOpenIdTokenForDeveloperIdentity tests that calling NewOpenIdTokenForDeveloperIdentity on a nil receiver returns an error
func TestNilReceiverNewOpenIdTokenForDeveloperIdentity(t *testing.T) {
	var c *Cognito
	identityId, token, err := c.NewOpenIdTokenForDeveloperIdentity("pool-id", "provider-id", "provider-name")
	if err == nil {
		t.Error("Expected error when calling NewOpenIdTokenForDeveloperIdentity on nil receiver, got nil")
	}
	if err.Error() != "Cognito receiver is nil" {
		t.Errorf("Expected 'Cognito receiver is nil', got '%s'", err.Error())
	}
	if identityId != "" {
		t.Errorf("Expected empty identityId, got %s", identityId)
	}
	if token != "" {
		t.Errorf("Expected empty token, got %s", token)
	}
}

// TestNilReceiverRefreshOpenIdTokenForDeveloperIdentity tests that calling RefreshOpenIdTokenForDeveloperIdentity on a nil receiver returns an error
func TestNilReceiverRefreshOpenIdTokenForDeveloperIdentity(t *testing.T) {
	var c *Cognito
	identityId, token, err := c.RefreshOpenIdTokenForDeveloperIdentity("pool-id", "provider-id", "provider-name", "existing-id")
	if err == nil {
		t.Error("Expected error when calling RefreshOpenIdTokenForDeveloperIdentity on nil receiver, got nil")
	}
	if err.Error() != "Cognito receiver is nil" {
		t.Errorf("Expected 'Cognito receiver is nil', got '%s'", err.Error())
	}
	if identityId != "" {
		t.Errorf("Expected empty identityId, got %s", identityId)
	}
	if token != "" {
		t.Errorf("Expected empty token, got %s", token)
	}
}
