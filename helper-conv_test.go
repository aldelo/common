package helper

/*
 * Copyright 2020-2026 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

// Executable specification for Float64ToCurrencyString.
//
// Float64ToCurrencyString is a DISPLAY-only helper. The godoc warning at
// helper-conv.go:417-429 was added as P0-12 of the v1.7.9 release-readiness
// remediation pass because the previous docstring ("Use for monetary
// amounts") invited downstream callers to treat float64 as if it were
// decimal arithmetic, which it is not.
//
// These tests encode the IEEE-754 drift hazards the godoc warns about as
// executable specs, so future readers can run the package and see the drift
// themselves, not just read a paragraph about it.
//
// Rule #10 (workspace): observable contract is preserved. The function still
// returns "%.2f". These tests do NOT pin the output — they pin the HAZARDS
// the output silently hides.

import (
	"testing"
)

// floatNoFold returns its argument unchanged, but through a function call
// so the Go compiler cannot fold the surrounding expression at arbitrary
// precision. Needed because `drifted := 0.1 + 0.2` is a CONSTANT expression
// evaluated at compile time with unbounded precision (then converted to
// float64), whereas `drifted := floatNoFold(0.1) + floatNoFold(0.2)` is a
// RUNTIME expression on typed float64 values — which is where IEEE-754
// rounding errors actually happen and where downstream callers will feel
// them in production.
//
//go:noinline
func floatNoFold(x float64) float64 { return x }

// -----------------------------------------------------------------------
// Hazard 1: direct binary-fraction drift (classic 0.1 + 0.2 case).
//
// 0.1 and 0.2 have no exact binary representation, so their runtime sum
// is not exactly 0.3. Float64ToCurrencyString rounds to 2 decimals and
// hides the discrepancy — both sums format as "0.30".
//
// Caller risk: if the caller later compares the un-formatted float to a
// "known" value like 0.3, the comparison fails — but the DISPLAYED receipt
// shows the "expected" amount, making the bug invisible on the UI.
// -----------------------------------------------------------------------
func TestFloat64ToCurrencyString_DisplayHidesBinaryFractionDrift(t *testing.T) {
	// Force runtime arithmetic on typed float64 — see floatNoFold comment.
	drifted := floatNoFold(0.1) + floatNoFold(0.2)
	exact := floatNoFold(0.3)

	// Invariant A: the two floats are NOT equal at the bit level. This is
	// the whole reason the godoc warning exists.
	if drifted == exact {
		t.Fatalf("test premise broken: 0.1+0.2 == 0.3 at runtime on this "+
			"platform; if this ever happens, IEEE-754 semantics have "+
			"changed and the godoc hazard example is stale "+
			"(got drifted=%v exact=%v)", drifted, exact)
	}

	// Invariant B: Float64ToCurrencyString nonetheless renders them the
	// same. This is precisely the hazard the godoc warns about — the
	// display masks a value that will fail == in subsequent arithmetic.
	gotDrifted := Float64ToCurrencyString(drifted)
	gotExact := Float64ToCurrencyString(exact)
	if gotDrifted != gotExact {
		t.Fatalf("display divergence: drifted=%q exact=%q — if these ever "+
			"differ, the godoc example at helper-conv.go:429 is wrong",
			gotDrifted, gotExact)
	}
	if gotDrifted != "0.30" {
		t.Fatalf("expected both renderings to be %q, got %q", "0.30", gotDrifted)
	}
}

// -----------------------------------------------------------------------
// Hazard 2: path-dependent total.
//
// Reaching the "same" monetary total via two different computation paths
// (sum-of-line-items vs. subtotal+tax) can produce float64 values that
// differ at the bit level but render identically via Float64ToCurrencyString.
//
// Caller risk: storing the displayed value in one place and the computed
// value in another, then later == comparing them, silently fails.
// -----------------------------------------------------------------------
func TestFloat64ToCurrencyString_DisplayHidesPathDependentTotals(t *testing.T) {
	// Force runtime arithmetic — constant literals would be folded exactly.
	// Path A: sum three line items of $0.10 each.
	pathA := floatNoFold(0.10) + floatNoFold(0.10) + floatNoFold(0.10)

	// Path B: one line item of $0.30.
	pathB := floatNoFold(0.30)

	// Display contract: both paths must format identically regardless of
	// whether the underlying float64 bits differ. This check always runs.
	dispA := Float64ToCurrencyString(pathA)
	dispB := Float64ToCurrencyString(pathB)
	if dispA != dispB {
		t.Fatalf("display divergence on path-dependent total: A=%q B=%q — if "+
			"these ever differ, the godoc 'sum-of-line-items vs. subtotal+tax' "+
			"example at helper-conv.go:423-424 is wrong",
			dispA, dispB)
	}

	// Informational: verify that bit-level drift exists for this input.
	// On IEEE-754 float64, 0.1+0.1+0.1 != 0.3 because intermediate rounding
	// differs. If a platform ever makes them equal, the display contract above
	// is still verified — we just log the anomaly.
	if pathA == pathB {
		t.Logf("note: path-dependent drift not observable for this input — "+
			"pathA=%v pathB=%v — display contract still verified above",
			pathA, pathB)
	}
}

// -----------------------------------------------------------------------
// Contract pin: the function still returns "%.2f" formatting.
//
// This is the observable contract downstream repos depend on. Rule #10
// forbids changing it in a minor bump. If anyone ever "fixes" the drift
// hazard by changing the format string, this test catches it.
// -----------------------------------------------------------------------
func TestFloat64ToCurrencyString_FormatContractIsTwoDecimals(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.00"},
		{1, "1.00"},
		{1.5, "1.50"},
		{1.005, "1.00"}, // banker's rounding on "%.2f" — documented Go behavior
		{-1.25, "-1.25"},
		{1234567.89, "1234567.89"},
	}
	for _, c := range cases {
		if got := Float64ToCurrencyString(c.in); got != c.want {
			t.Errorf("Float64ToCurrencyString(%v) = %q, want %q "+
				"— contract break, downstream receipts may change format",
				c.in, got, c.want)
		}
	}
}
