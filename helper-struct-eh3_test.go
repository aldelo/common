package helper

/*
 * Copyright 2020-2026 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

// Tests for EH-3 fix: ParseInt32/ParseBool parse failures in struct-tag parsing
// are now logged instead of silently discarded.

import (
	"bytes"
	"log"
	"reflect"
	"strings"
	"testing"
)

// captureLog temporarily redirects the standard logger output to a buffer,
// runs fn, then restores the original writer. Returns everything the standard
// logger emitted during fn.
func captureLog(fn func()) string {
	var buf bytes.Buffer
	orig := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0) // drop timestamp so output is deterministic
	defer func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	}()
	fn()
	return buf.String()
}

// --- parseInt32Logged ---

func TestParseInt32Logged_ValidInput(t *testing.T) {
	out := captureLog(func() {
		v := parseInt32Logged("42", "size", "TestField")
		if v != 42 {
			t.Fatalf("expected 42, got %d", v)
		}
	})
	if len(out) > 0 {
		t.Fatalf("expected no log output for valid input, got: %s", out)
	}
}

func TestParseInt32Logged_InvalidInput(t *testing.T) {
	out := captureLog(func() {
		v := parseInt32Logged("abc", "size", "TestField")
		if v != 0 {
			t.Fatalf("expected 0 for invalid input, got %d", v)
		}
	})
	if !strings.Contains(out, "[WARN]") {
		t.Fatalf("expected [WARN] in log output, got: %s", out)
	}
	if !strings.Contains(out, "ParseInt32 failed") {
		t.Fatalf("expected 'ParseInt32 failed' in log output, got: %s", out)
	}
	if !strings.Contains(out, `size="abc"`) {
		t.Fatalf("expected tag name and value in log output, got: %s", out)
	}
	if !strings.Contains(out, "TestField") {
		t.Fatalf("expected field context in log output, got: %s", out)
	}
}

func TestParseInt32Logged_EmptyInput_NoWarn(t *testing.T) {
	// Empty string is a valid "nothing" — should not warn.
	out := captureLog(func() {
		v := parseInt32Logged("", "size", "TestField")
		if v != 0 {
			t.Fatalf("expected 0 for empty input, got %d", v)
		}
	})
	if len(out) > 0 {
		t.Fatalf("expected no log output for empty input, got: %s", out)
	}
}

// --- parseBoolLogged ---

func TestParseBoolLogged_ValidInput(t *testing.T) {
	out := captureLog(func() {
		v := parseBoolLogged("true", "skipblank", "TestField")
		if !v {
			t.Fatal("expected true for 'true' input")
		}
	})
	if len(out) > 0 {
		t.Fatalf("expected no log output for valid input, got: %s", out)
	}
}

func TestParseBoolLogged_InvalidInput(t *testing.T) {
	out := captureLog(func() {
		v := parseBoolLogged("notabool", "skipblank", "TestField")
		if v {
			t.Fatal("expected false for invalid input")
		}
	})
	if !strings.Contains(out, "[WARN]") {
		t.Fatalf("expected [WARN] in log output, got: %s", out)
	}
	if !strings.Contains(out, "ParseBool failed") {
		t.Fatalf("expected 'ParseBool failed' in log output, got: %s", out)
	}
}

func TestParseBoolLogged_EmptyInput_NoWarn(t *testing.T) {
	out := captureLog(func() {
		v := parseBoolLogged("", "skipblank", "TestField")
		if v {
			t.Fatal("expected false for empty input")
		}
	})
	if len(out) > 0 {
		t.Fatalf("expected no log output for empty input, got: %s", out)
	}
}

// --- Integration: csvParseFieldConfig with invalid tags ---

// testCsvStruct is a synthetic struct with deliberately invalid tag values
// to exercise the EH-3 fix in csvParseFieldConfig.
type testCsvStructInvalidSize struct {
	Field1 string `pos:"0" type:"a" size:"abc"`
}

func TestCsvParseFieldConfig_InvalidSize_Logs(t *testing.T) {
	field := reflect.TypeOf(testCsvStructInvalidSize{}).Field(0)
	out := captureLog(func() {
		cfg, ok := csvParseFieldConfig(field)
		if !ok {
			t.Fatal("expected ok=true (pos is valid)")
		}
		// Invalid size should default to 0
		if cfg.sizeMin != 0 || cfg.sizeMax != 0 {
			t.Fatalf("expected sizeMin=0, sizeMax=0 for invalid size, got min=%d max=%d", cfg.sizeMin, cfg.sizeMax)
		}
	})
	if !strings.Contains(out, "[WARN]") {
		t.Fatalf("expected [WARN] in log for invalid size tag, got: %s", out)
	}
	if !strings.Contains(out, "Field1") {
		t.Fatalf("expected field name in log context, got: %s", out)
	}
}

type testCsvStructInvalidRange struct {
	Field2 string `pos:"0" type:"n" range:"xyz"`
}

func TestCsvParseFieldConfig_InvalidRange_Logs(t *testing.T) {
	field := reflect.TypeOf(testCsvStructInvalidRange{}).Field(0)
	out := captureLog(func() {
		cfg, ok := csvParseFieldConfig(field)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if cfg.rangeMin != 0 {
			t.Fatalf("expected rangeMin=0 for invalid range, got %d", cfg.rangeMin)
		}
	})
	if !strings.Contains(out, "[WARN]") {
		t.Fatalf("expected [WARN] in log for invalid range tag, got: %s", out)
	}
}

type testCsvStructInvalidBool struct {
	Field3 string `pos:"0" type:"a" skipblank:"notbool" skipzero:"notbool" zeroblank:"notbool"`
}

func TestCsvParseFieldConfig_InvalidBool_Logs(t *testing.T) {
	field := reflect.TypeOf(testCsvStructInvalidBool{}).Field(0)
	out := captureLog(func() {
		cfg, ok := csvParseFieldConfig(field)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if cfg.skipBlank || cfg.skipZero || cfg.zeroBlank {
			t.Fatal("expected all bools to be false for invalid input")
		}
	})
	if !strings.Contains(out, "ParseBool failed") {
		t.Fatalf("expected ParseBool warnings in log, got: %s", out)
	}
	// Should have 3 separate warnings
	if count := strings.Count(out, "[WARN]"); count != 3 {
		t.Fatalf("expected 3 [WARN] lines for 3 invalid bools, got %d", count)
	}
}

// --- Integration: csvParseFieldConfig with VALID tags (no log output) ---

type testCsvStructValidTags struct {
	Field4 string `pos:"0" type:"a" size:"5..10" range:"1..100" skipblank:"true" skipzero:"false" zeroblank:"true"`
}

func TestCsvParseFieldConfig_ValidTags_NoLog(t *testing.T) {
	field := reflect.TypeOf(testCsvStructValidTags{}).Field(0)
	out := captureLog(func() {
		cfg, ok := csvParseFieldConfig(field)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if cfg.sizeMin != 5 || cfg.sizeMax != 10 {
			t.Fatalf("expected sizeMin=5, sizeMax=10, got min=%d max=%d", cfg.sizeMin, cfg.sizeMax)
		}
		if cfg.rangeMin != 1 || cfg.rangeMax != 100 {
			t.Fatalf("expected rangeMin=1, rangeMax=100, got min=%d max=%d", cfg.rangeMin, cfg.rangeMax)
		}
		if !cfg.skipBlank || cfg.skipZero || !cfg.zeroBlank {
			t.Fatalf("unexpected bool values: skipBlank=%v skipZero=%v zeroBlank=%v", cfg.skipBlank, cfg.skipZero, cfg.zeroBlank)
		}
	})
	if len(out) > 0 {
		t.Fatalf("expected no log output for valid tags, got: %s", out)
	}
}
