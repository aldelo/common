package bedrockruntime

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

// Pins the InvokeModel call-deadline fix. Previously InvokeModel called the SDK
// with context.Background() on the non-xray path (and the bare xray segment ctx
// otherwise) — NO timeout. A hung Bedrock endpoint (AWS outage / network
// partition) parked the caller's goroutine indefinitely; enough concurrent hung
// calls exhaust the task's goroutines. This mirrors the fix already shipped in
// wrapper/sns and wrapper/kms (defaultSNSCallTimeout / defaultKMSCallTimeout).
//
// The deadline-building is factored into bedrockCallCtx so it can be pinned
// without a live Bedrock client (the wrapper exposes no client seam to mock the
// actual InvokeModel call).

import (
	"context"
	"testing"
	"time"
)

// TestBedrockCallCtx_AlwaysHasDeadline verifies the call context carries a
// deadline on BOTH paths (xray segment set or not) — the property whose absence
// was the bug.
func TestBedrockCallCtx_AlwaysHasDeadline(t *testing.T) {
	t.Run("no_xray_segment", func(t *testing.T) {
		ctx, cancel := bedrockCallCtx(nil, false, defaultBedrockCallTimeout)
		defer cancel()
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatal("no deadline when segCtx unset — a hung Bedrock call would park the goroutine forever")
		}
		if until := time.Until(dl); until <= 0 || until > defaultBedrockCallTimeout+time.Second {
			t.Errorf("deadline in %v, want within (0, %v]", until, defaultBedrockCallTimeout)
		}
	})

	t.Run("xray_segment_set_preserves_lineage_and_deadline", func(t *testing.T) {
		type ctxKey string
		parent := context.WithValue(context.Background(), ctxKey("seg"), "x")
		ctx, cancel := bedrockCallCtx(parent, true, defaultBedrockCallTimeout)
		defer cancel()
		if _, ok := ctx.Deadline(); !ok {
			t.Error("no deadline when segCtxSet — xray-on path must also be bounded")
		}
		if ctx.Value(ctxKey("seg")) != "x" {
			t.Error("segment ctx lineage not preserved (deadline must wrap the segment parent)")
		}
	})
}

// TestBedrockCallTimeout_ContractPin locks the chosen deadline. Bedrock model
// invokes can legitimately take up to ~2 minutes for large prompts, so the bound
// is generous (vs SNS's 30s control-plane calls) — changing it is a deliberate
// decision, not an accidental refactor.
func TestBedrockCallTimeout_ContractPin(t *testing.T) {
	if defaultBedrockCallTimeout != 120*time.Second {
		t.Errorf("defaultBedrockCallTimeout = %v, want 120s", defaultBedrockCallTimeout)
	}
}

// TestBedrockRuntime_CallTimeout_ZeroUsesDefault pins the opt-in contract: a
// zero-value CallTimeout (the default for every existing caller) resolves to
// defaultBedrockCallTimeout, so adding the field changes nothing on a version
// bump.
func TestBedrockRuntime_CallTimeout_ZeroUsesDefault(t *testing.T) {
	s := &BedrockRuntime{}
	if got := s.callTimeout(); got != defaultBedrockCallTimeout {
		t.Errorf("callTimeout() with zero CallTimeout = %v, want default %v", got, defaultBedrockCallTimeout)
	}
}

// TestBedrockRuntime_CallTimeout_ConfiguredHonored proves a consumer can override
// the 120s default (e.g. a model/prompt that legitimately needs longer, or a
// caller with a tighter budget) and that bedrockCallCtx applies the override
// rather than the default.
func TestBedrockRuntime_CallTimeout_ConfiguredHonored(t *testing.T) {
	s := &BedrockRuntime{CallTimeout: 7 * time.Second}
	if got := s.callTimeout(); got != 7*time.Second {
		t.Fatalf("callTimeout() = %v, want 7s", got)
	}

	ctx, cancel := bedrockCallCtx(nil, false, s.callTimeout())
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("configured call ctx has no deadline")
	}
	// deadline must reflect the 7s override, NOT the 120s default
	if until := time.Until(dl); until <= 6*time.Second || until > 7*time.Second+time.Second {
		t.Errorf("deadline in %v, want ~7s (the configured override, not the 120s default)", until)
	}
}
