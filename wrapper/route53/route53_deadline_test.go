package route53

import (
	"context"
	"testing"
	"time"
)

func TestEnsureRoute53Ctx_WithSegCtx_Gets30sDeadline(t *testing.T) {
	parent := context.Background()
	ctx, cancel := ensureRoute53Ctx(parent)
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

func TestEnsureRoute53Ctx_NilSegCtx_Gets30sDeadline(t *testing.T) {
	ctx, cancel := ensureRoute53Ctx(nil)
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

func TestEnsureRoute53Ctx_ParentDeadlineShorter_Wins(t *testing.T) {
	// If the parent context already has a shorter deadline, it should
	// be preserved (context.WithTimeout returns the earlier of the
	// two deadlines).
	parent, parentCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer parentCancel()

	ctx, cancel := ensureRoute53Ctx(parent)
	defer cancel()

	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	remaining := time.Until(dl)
	// Parent's 5s deadline is shorter than the 30s default, so it wins
	if remaining > 6*time.Second || remaining < 4*time.Second {
		t.Fatalf("expected ~5s deadline (parent wins), got %v", remaining)
	}
}

func TestEnsureRoute53Ctx_CancelActuallyCancels(t *testing.T) {
	ctx, cancel := ensureRoute53Ctx(context.Background())
	cancel()

	select {
	case <-ctx.Done():
		// expected
	default:
		t.Fatal("expected context to be cancelled after cancel()")
	}
}
