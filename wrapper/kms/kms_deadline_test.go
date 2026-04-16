package kms

import (
	"context"
	"testing"
	"time"
)

func TestEnsureKMSCtx_WithSegCtx_Gets30sDeadline(t *testing.T) {
	parent := context.Background()
	ctx, cancel := ensureKMSCtx(parent)
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	remaining := time.Until(dl)
	if remaining > 31*time.Second || remaining < 29*time.Second {
		t.Fatalf("expected ~30s deadline, got %v", remaining)
	}
}

func TestEnsureKMSCtx_NilSegCtx_Gets30sDeadline(t *testing.T) {
	ctx, cancel := ensureKMSCtx(nil)
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	remaining := time.Until(dl)
	if remaining > 31*time.Second || remaining < 29*time.Second {
		t.Fatalf("expected ~30s deadline, got %v", remaining)
	}
}

func TestEnsureKMSCtx_ParentDeadlineShorter_Wins(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer parentCancel()
	ctx, cancel := ensureKMSCtx(parent)
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	remaining := time.Until(dl)
	if remaining > 6*time.Second || remaining < 4*time.Second {
		t.Fatalf("expected ~5s deadline (parent wins), got %v", remaining)
	}
}

func TestEnsureKMSCtx_CancelActuallyCancels(t *testing.T) {
	ctx, cancel := ensureKMSCtx(context.Background())
	cancel()
	select {
	case <-ctx.Done():
		// expected
	default:
		t.Fatal("expected context to be cancelled after cancel()")
	}
}
