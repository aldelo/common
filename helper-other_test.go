package helper

/*
 * Copyright 2020-2026 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

// Executable specification for SliceDeleteElement.
//
// This function previously had ZERO unit tests, which is how a
// reflect.Value.Set-using-unaddressable-value panic shipped in v1.7.8:
// the "settable copy" fallback used reflect.MakeSlice, which does not
// produce an addressable Value. Any call with a VALUE-type slice and
// any non-last, non-out-of-bounds index triggered the panic -- this
// is literally the most common call pattern.
//
// These tests pin the documented contract from the function's godoc:
//   - negative index counts from the right (-1 = last, -2 = 2nd-last)
//   - out-of-bounds index returns the slice unchanged
//   - nil input returns nil
//   - empty slice returns empty
//   - pointer-to-slice form mutates the caller's slice in-place
//   - order is NOT preserved (swap-with-last-then-trim algorithm)
//
// Rule #10 (workspace): observable contract is what the godoc promises,
// not what the buggy implementation happened to do.

import (
	"reflect"
	"sort"
	"testing"
)

// intSetEqual compares two []int as multisets -- order does not matter.
// Used because SliceDeleteElement's swap-then-trim algorithm does not
// preserve original ordering.
func intSetEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]int(nil), a...)
	bb := append([]int(nil), b...)
	sort.Ints(aa)
	sort.Ints(bb)
	return reflect.DeepEqual(aa, bb)
}

// -----------------------------------------------------------------------
// Negative index (the panic-path regression tests)
// -----------------------------------------------------------------------

func TestSliceDeleteElement_ValueSlice_NegOne(t *testing.T) {
	// Pre-fix: reflect.Value.Set using unaddressable value panic
	got := SliceDeleteElement([]int{1, 2, 3}, -1)
	out, ok := got.([]int)
	if !ok {
		t.Fatalf("expected []int, got %T", got)
	}
	if len(out) != 2 {
		t.Fatalf("expected len 2, got %d (%v)", len(out), out)
	}
	if !intSetEqual(out, []int{1, 2}) {
		t.Errorf("expected multiset {1,2}, got %v", out)
	}
}

func TestSliceDeleteElement_ValueSlice_NegTwo(t *testing.T) {
	got := SliceDeleteElement([]int{1, 2, 3}, -2)
	out := got.([]int)
	if len(out) != 2 {
		t.Fatalf("expected len 2, got %d (%v)", len(out), out)
	}
	if !intSetEqual(out, []int{1, 3}) {
		t.Errorf("expected multiset {1,3}, got %v", out)
	}
}

func TestSliceDeleteElement_ValueSlice_NegFull(t *testing.T) {
	// -len removes the first element (index 0)
	got := SliceDeleteElement([]int{10, 20, 30}, -3)
	out := got.([]int)
	if !intSetEqual(out, []int{20, 30}) {
		t.Errorf("expected multiset {20,30}, got %v", out)
	}
}

// -----------------------------------------------------------------------
// Positive index (happy path)
// -----------------------------------------------------------------------

func TestSliceDeleteElement_ValueSlice_PosFirst(t *testing.T) {
	got := SliceDeleteElement([]int{1, 2, 3, 4}, 0)
	out := got.([]int)
	if !intSetEqual(out, []int{2, 3, 4}) {
		t.Errorf("expected multiset {2,3,4}, got %v", out)
	}
}

func TestSliceDeleteElement_ValueSlice_PosMiddle(t *testing.T) {
	got := SliceDeleteElement([]int{1, 2, 3, 4, 5}, 2)
	out := got.([]int)
	if len(out) != 4 {
		t.Fatalf("expected len 4, got %d", len(out))
	}
	// element at index 2 is 3 -- should be gone
	for _, v := range out {
		if v == 3 {
			t.Errorf("element 3 should have been removed, got %v", out)
		}
	}
}

func TestSliceDeleteElement_ValueSlice_PosLast(t *testing.T) {
	got := SliceDeleteElement([]int{1, 2, 3}, 2)
	out := got.([]int)
	if !intSetEqual(out, []int{1, 2}) {
		t.Errorf("expected multiset {1,2}, got %v", out)
	}
}

// -----------------------------------------------------------------------
// Out-of-bounds cases (must leave slice unchanged, per godoc)
// -----------------------------------------------------------------------

func TestSliceDeleteElement_OutOfBoundsPositive(t *testing.T) {
	got := SliceDeleteElement([]int{1, 2, 3}, 99)
	out := got.([]int)
	if !intSetEqual(out, []int{1, 2, 3}) {
		t.Errorf("expected unchanged {1,2,3}, got %v", out)
	}
}

func TestSliceDeleteElement_OutOfBoundsNegative(t *testing.T) {
	got := SliceDeleteElement([]int{1, 2, 3}, -99)
	out := got.([]int)
	if !intSetEqual(out, []int{1, 2, 3}) {
		t.Errorf("expected unchanged {1,2,3}, got %v", out)
	}
}

// -----------------------------------------------------------------------
// Edge cases
// -----------------------------------------------------------------------

func TestSliceDeleteElement_EmptySlice(t *testing.T) {
	got := SliceDeleteElement([]int{}, 0)
	// Per function impl, empty slices return a zero-value shape.
	out, ok := got.([]int)
	if !ok {
		t.Fatalf("expected []int, got %T", got)
	}
	if len(out) != 0 {
		t.Errorf("expected empty slice, got %v", out)
	}
}

func TestSliceDeleteElement_SingleElement_NegOne(t *testing.T) {
	got := SliceDeleteElement([]int{42}, -1)
	out, ok := got.([]int)
	if !ok {
		t.Fatalf("expected []int, got %T", got)
	}
	if len(out) != 0 {
		t.Errorf("expected empty slice after removing only element, got %v", out)
	}
}

func TestSliceDeleteElement_SingleElement_PosZero(t *testing.T) {
	got := SliceDeleteElement([]int{42}, 0)
	out := got.([]int)
	if len(out) != 0 {
		t.Errorf("expected empty slice, got %v", out)
	}
}

func TestSliceDeleteElement_NilInput(t *testing.T) {
	got := SliceDeleteElement(nil, -1)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestSliceDeleteElement_NonSliceInput(t *testing.T) {
	// Function contract: non-slice input returns the input unchanged
	in := 42
	got := SliceDeleteElement(in, 0)
	if got != 42 {
		t.Errorf("expected 42 unchanged, got %v", got)
	}
}

// -----------------------------------------------------------------------
// Pointer-to-slice form (mutates caller)
// -----------------------------------------------------------------------

func TestSliceDeleteElement_PointerSlice_NegOne(t *testing.T) {
	sl := []string{"a", "b", "c"}
	SliceDeleteElement(&sl, -1)
	if len(sl) != 2 {
		t.Fatalf("expected caller slice shortened to len 2, got %d (%v)", len(sl), sl)
	}
	// Algorithm doesn't preserve order; verify the removed element is gone
	// and the remaining elements are the original first two (in any order).
	seen := map[string]bool{}
	for _, v := range sl {
		seen[v] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Errorf("expected {a,b} to remain, got %v", sl)
	}
}

func TestSliceDeleteElement_PointerSlice_PosZero(t *testing.T) {
	sl := []int{10, 20, 30}
	SliceDeleteElement(&sl, 0)
	if len(sl) != 2 {
		t.Fatalf("expected len 2, got %d", len(sl))
	}
	// element 10 should be gone
	for _, v := range sl {
		if v == 10 {
			t.Errorf("10 should have been removed, got %v", sl)
		}
	}
}

func TestSliceDeleteElement_NilPointer(t *testing.T) {
	var sl *[]int
	got := SliceDeleteElement(sl, 0)
	if got != nil {
		t.Errorf("expected nil for nil pointer input, got %v", got)
	}
}

// -----------------------------------------------------------------------
// Complex element types (exercise reflect.Swapper on structs)
// -----------------------------------------------------------------------

func TestSliceDeleteElement_StructSlice(t *testing.T) {
	type Item struct {
		ID   int
		Name string
	}
	in := []Item{
		{ID: 1, Name: "a"},
		{ID: 2, Name: "b"},
		{ID: 3, Name: "c"},
	}
	got := SliceDeleteElement(in, -1)
	out := got.([]Item)
	if len(out) != 2 {
		t.Fatalf("expected len 2, got %d", len(out))
	}
	// Element with Name "c" should be gone
	for _, it := range out {
		if it.Name == "c" {
			t.Errorf("expected 'c' removed, got %v", out)
		}
	}
}

// -----------------------------------------------------------------------
// CMN-C1 / CMN-C2 regression tests — caller-backing mutation via named variables
// -----------------------------------------------------------------------
//
// The v1.7.8 contract: SliceDeleteElement on a VALUE-form slice returns
// a new slice with fresh backing and does NOT mutate the caller's slice.
// v1.7.10 silently regressed this (reflect.New(t).Elem() + Set copies
// only the header, so Swapper operated on caller memory).
//
// The prior test suite used inline literal arguments exclusively
// (`SliceDeleteElement([]int{1,2,3}, 0)`), which made mutation
// unobservable by construction. These tests pin the contract against a
// NAMED local variable so a future regression is caught the moment it
// lands.

func TestSliceDeleteElement_ValueSlice_DoesNotMutateCaller_Int(t *testing.T) {
	original := []int{10, 20, 30, 40}
	snapshot := append([]int(nil), original...)

	_ = SliceDeleteElement(original, 1)

	if !reflect.DeepEqual(original, snapshot) {
		t.Errorf("caller's backing array was mutated:\n  got:  %v\n  want: %v",
			original, snapshot)
	}
}

func TestSliceDeleteElement_ValueSlice_DoesNotMutateCaller_String(t *testing.T) {
	original := []string{"alpha", "bravo", "charlie", "delta"}
	snapshot := append([]string(nil), original...)

	_ = SliceDeleteElement(original, 0)

	if !reflect.DeepEqual(original, snapshot) {
		t.Errorf("caller's backing array was mutated:\n  got:  %v\n  want: %v",
			original, snapshot)
	}
}

func TestSliceDeleteElement_ValueSlice_DoesNotMutateCaller_Struct(t *testing.T) {
	type Item struct {
		ID   int
		Name string
	}
	original := []Item{
		{ID: 1, Name: "a"},
		{ID: 2, Name: "b"},
		{ID: 3, Name: "c"},
		{ID: 4, Name: "d"},
	}
	snapshot := append([]Item(nil), original...)

	_ = SliceDeleteElement(original, 2)

	if !reflect.DeepEqual(original, snapshot) {
		t.Errorf("caller's backing array was mutated:\n  got:  %v\n  want: %v",
			original, snapshot)
	}
}

func TestSliceDeleteElement_ValueSlice_DoesNotMutateCaller_NegativeIndex(t *testing.T) {
	// Negative index path — ensure MakeSlice branch fires on neg-idx too.
	original := []int{100, 200, 300, 400, 500}
	snapshot := append([]int(nil), original...)

	_ = SliceDeleteElement(original, -2)

	if !reflect.DeepEqual(original, snapshot) {
		t.Errorf("caller's backing array was mutated on neg-idx path:\n  got:  %v\n  want: %v",
			original, snapshot)
	}
}

func TestSliceDeleteElement_ValueSlice_ResultBackingIsDisjoint(t *testing.T) {
	// Stronger invariant: mutating the RESULT must not affect the caller.
	original := []int{1, 2, 3, 4, 5}
	snapshot := append([]int(nil), original...)

	got := SliceDeleteElement(original, 0)
	out := got.([]int)

	// Stomp every element of the result.
	for i := range out {
		out[i] = -999
	}

	if !reflect.DeepEqual(original, snapshot) {
		t.Errorf("stomping result mutated caller's backing array:\n  got:  %v\n  want: %v",
			original, snapshot)
	}
}

func TestSliceDeleteElement_PointerSlice_StillMutatesInPlace(t *testing.T) {
	// Pointer-form callers deliberately expect in-place mutation — this
	// is the documented contract from the godoc. The CMN-C1 fix must NOT
	// break this path.
	in := []int{1, 2, 3, 4}
	_ = SliceDeleteElement(&in, 0)

	if len(in) != 3 {
		t.Errorf("pointer-form did not mutate caller in place: len(in)=%d want 3", len(in))
	}
	// Element at index 0 was swapped with last (4) then trimmed,
	// so remaining multiset should be {2,3,4} (order not preserved).
	if !intSetEqual(in, []int{2, 3, 4}) {
		t.Errorf("pointer-form multiset wrong: got %v want {2,3,4}", in)
	}
}

// -----------------------------------------------------------------------
// Unicode / rune boundary coverage (C1-004)
// -----------------------------------------------------------------------
//
// Close a gap in the pre-existing test matrix: none of the prior cases
// exercised the helper with elements whose in-memory representation is
// a multi-byte UTF-8 sequence (for []string) or a supplementary-plane
// rune (for []rune). The reflect path in SliceDeleteElement operates at
// the element level, not the byte level, so it *should* be indifferent
// to the element's internal encoding — but "should" is not "proven".
//
// This test pins the element-level contract: for every boundary
// position (0, middle, len-1), deleting a multi-byte element yields a
// result whose remaining elements exactly match the expected multiset,
// with no mis-slicing at byte boundaries and no rune corruption.
//
// Scope: []string (Go strings are already rune-opaque byte sequences,
// so this catches any accidental byte-level handling in the helper)
// and []rune (each element is a full int32 code point, including
// supplementary-plane values above U+FFFF). []byte is intentionally
// NOT covered here — it is semantically a byte buffer, not a Unicode
// sequence, and the existing int/string/struct tests already prove
// primitive-slice handling.

// strSetEqual compares two []string as multisets — order does not matter.
// SliceDeleteElement's swap-with-last algorithm does not preserve order.
func strSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	return reflect.DeepEqual(aa, bb)
}

// runeSetEqual compares two []rune as multisets — order does not matter.
func runeSetEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]rune(nil), a...)
	bb := append([]rune(nil), b...)
	sort.Slice(aa, func(i, j int) bool { return aa[i] < aa[j] })
	sort.Slice(bb, func(i, j int) bool { return bb[i] < bb[j] })
	return reflect.DeepEqual(aa, bb)
}

func TestSliceDeleteElement_UnicodeBoundary(t *testing.T) {
	// 4-byte UTF-8 characters (supplementary-plane code points):
	//   "🚀" U+1F680 ROCKET              — 4 bytes in UTF-8
	//   "𝕏" U+1D54F MATH DOUBLE-STRUCK X — 4 bytes in UTF-8
	//   "你" U+4F60  CJK                  — 3 bytes in UTF-8
	//   "é" U+00E9  LATIN SMALL E ACUTE  — 2 bytes in UTF-8
	// Mixed widths ensure any byte-level indexing bug would surface as a
	// length mismatch or a corrupted string in the result.
	original := []string{"🚀", "𝕏", "你", "é", "ascii"}

	type tc struct {
		name   string
		idx    int
		remain []string // expected remaining multiset
	}
	cases := []tc{
		{"first", 0, []string{"𝕏", "你", "é", "ascii"}},
		{"middle", 2, []string{"🚀", "𝕏", "é", "ascii"}},
		{"last", len(original) - 1, []string{"🚀", "𝕏", "你", "é"}},
		{"neg_one_last", -1, []string{"🚀", "𝕏", "你", "é"}},
	}

	for _, c := range cases {
		t.Run("string_"+c.name, func(t *testing.T) {
			// Fresh copy per sub-test so one deletion does not affect the next.
			in := append([]string(nil), original...)
			got := SliceDeleteElement(in, c.idx)
			out, ok := got.([]string)
			if !ok {
				t.Fatalf("expected []string, got %T", got)
			}
			if len(out) != len(c.remain) {
				t.Fatalf("length mismatch: got %d want %d (got=%v)", len(out), len(c.remain), out)
			}
			if !strSetEqual(out, c.remain) {
				t.Errorf("multiset mismatch:\n  got:  %v\n  want: %v", out, c.remain)
			}
			// Defensive: verify every returned element is still a valid,
			// non-truncated string by confirming it round-trips through []rune.
			for i, s := range out {
				rs := []rune(s)
				if string(rs) != s {
					t.Errorf("result[%d] failed string<->rune round trip: %q", i, s)
				}
			}
		})
	}

	// []rune form: supplementary-plane code points. Each element is a
	// single int32, so this test proves the helper treats rune slices
	// element-wise (it should — it's a reflect.Slice path, not a byte path).
	//
	//   U+1F680 ROCKET              (0x1F680)
	//   U+1D54F MATH DOUBLE-STRUCK X (0x1D54F)
	//   U+4F60  你                   (0x4F60)
	//   U+00E9  é                    (0x00E9)
	//   U+0041  A                    (0x0041)
	runeOriginal := []rune{0x1F680, 0x1D54F, 0x4F60, 0x00E9, 0x0041}

	runeCases := []struct {
		name   string
		idx    int
		remain []rune
	}{
		{"first", 0, []rune{0x1D54F, 0x4F60, 0x00E9, 0x0041}},
		{"middle", 2, []rune{0x1F680, 0x1D54F, 0x00E9, 0x0041}},
		{"last", len(runeOriginal) - 1, []rune{0x1F680, 0x1D54F, 0x4F60, 0x00E9}},
		{"neg_one_last", -1, []rune{0x1F680, 0x1D54F, 0x4F60, 0x00E9}},
	}

	for _, c := range runeCases {
		t.Run("rune_"+c.name, func(t *testing.T) {
			in := append([]rune(nil), runeOriginal...)
			got := SliceDeleteElement(in, c.idx)
			out, ok := got.([]rune)
			if !ok {
				t.Fatalf("expected []rune, got %T", got)
			}
			if len(out) != len(c.remain) {
				t.Fatalf("length mismatch: got %d want %d (got=%v)", len(out), len(c.remain), out)
			}
			if !runeSetEqual(out, c.remain) {
				t.Errorf("multiset mismatch:\n  got:  %v\n  want: %v", out, c.remain)
			}
			// Every returned rune must still be a valid supplementary-plane
			// code point — no truncation to BMP, no sign mangling.
			for i, r := range out {
				if r < 0 {
					t.Errorf("result[%d] rune went negative: %d", i, r)
				}
			}
		})
	}
}
