package bedrockruntime

import (
	"testing"

	"github.com/aldelo/common/wrapper/xray"
)

// TestNilReceiverConnect tests that calling Connect on a nil receiver returns an error
func TestNilReceiverConnect(t *testing.T) {
	var br *BedrockRuntime
	err := br.Connect()
	if err == nil {
		t.Error("Expected error when calling Connect on nil receiver, got nil")
	}
	if err.Error() != "BedrockRuntime receiver is nil" {
		t.Errorf("Expected 'BedrockRuntime receiver is nil', got '%s'", err.Error())
	}
}

// TestNilReceiverConnectWithParentSegment tests that calling Connect with parent segment on a nil receiver returns an error
func TestNilReceiverConnectWithParentSegment(t *testing.T) {
	var br *BedrockRuntime
	var seg *xray.XRayParentSegment
	err := br.Connect(seg)
	if err == nil {
		t.Error("Expected error when calling Connect on nil receiver, got nil")
	}
	if err.Error() != "BedrockRuntime receiver is nil" {
		t.Errorf("Expected 'BedrockRuntime receiver is nil', got '%s'", err.Error())
	}
}

// TestNilReceiverDisconnect tests that calling Disconnect on a nil receiver doesn't panic
func TestNilReceiverDisconnect(t *testing.T) {
	var br *BedrockRuntime
	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Disconnect panicked on nil receiver: %v", r)
		}
	}()
	br.Disconnect()
}

// TestNilReceiverUpdateParentSegment tests that calling UpdateParentSegment on a nil receiver doesn't panic
func TestNilReceiverUpdateParentSegment(t *testing.T) {
	var br *BedrockRuntime
	var seg *xray.XRayParentSegment
	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("UpdateParentSegment panicked on nil receiver: %v", r)
		}
	}()
	br.UpdateParentSegment(seg)
}

// TestNilReceiverInvokeModel tests that calling InvokeModel on a nil receiver returns an error
func TestNilReceiverInvokeModel(t *testing.T) {
	var br *BedrockRuntime
	output, err := br.InvokeModel("test-model", []byte("test-body"))
	if err == nil {
		t.Error("Expected error when calling InvokeModel on nil receiver, got nil")
	}
	if err.Error() != "BedrockRuntime receiver is nil" {
		t.Errorf("Expected 'BedrockRuntime receiver is nil', got '%s'", err.Error())
	}
	if output != nil {
		t.Errorf("Expected nil output, got %v", output)
	}
}
