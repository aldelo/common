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
