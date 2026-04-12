package ginhttpmethod

import "testing"

// allValues enumerates every defined GinHttpMethod constant.
var allValues = []struct {
	val  GinHttpMethod
	name string
	key  string
	iota int
}{
	{UNKNOWN, "UNKNOWN", "UNKNOWN", 0},
	{GET, "GET", "GET", 1},
	{POST, "POST", "POST", 2},
	{PUT, "PUT", "PUT", 3},
	{DELETE, "DELETE", "DELETE", 4},
}

func TestValueSliceCompleteness(t *testing.T) {
	vs := UNKNOWN.ValueSlice()
	if len(vs) != len(allValues) {
		t.Fatalf("ValueSlice length = %d, want %d", len(vs), len(allValues))
	}
	m := make(map[GinHttpMethod]bool, len(vs))
	for _, v := range vs {
		m[v] = true
	}
	for _, tc := range allValues {
		if !m[tc.val] {
			t.Errorf("ValueSlice missing %s (%d)", tc.name, tc.val)
		}
	}
}

func TestStringRoundTrip(t *testing.T) {
	for _, tc := range allValues {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.val.ParseByName(tc.val.String())
			if err != nil {
				t.Fatalf("ParseByName(%q) error: %v", tc.val.String(), err)
			}
			if got != tc.val {
				t.Errorf("ParseByName(%q) = %d, want %d", tc.val.String(), got, tc.val)
			}
		})
	}
}

func TestKeyRoundTrip(t *testing.T) {
	for _, tc := range allValues {
		t.Run(tc.key, func(t *testing.T) {
			got, err := tc.val.ParseByKey(tc.val.Key())
			if err != nil {
				t.Fatalf("ParseByKey(%q) error: %v", tc.val.Key(), err)
			}
			if got != tc.val {
				t.Errorf("ParseByKey(%q) = %d, want %d", tc.val.Key(), got, tc.val)
			}
		})
	}
}

func TestValid(t *testing.T) {
	for _, tc := range allValues {
		if !tc.val.Valid() {
			t.Errorf("%s (%d) should be Valid", tc.name, tc.val)
		}
	}
	invalid := GinHttpMethod(-1)
	if invalid.Valid() {
		t.Error("GinHttpMethod(-1) should not be Valid")
	}
}

func TestParseByNameInvalid(t *testing.T) {
	_, err := UNKNOWN.ParseByName("PATCH")
	if err == nil {
		t.Error("ParseByName(\"PATCH\") should return error")
	}
}

func TestParseByKeyInvalid(t *testing.T) {
	_, err := UNKNOWN.ParseByKey("PATCH")
	if err == nil {
		t.Error("ParseByKey(\"PATCH\") should return error")
	}
}

func TestIntValueSpotCheck(t *testing.T) {
	if DELETE.IntValue() != 4 {
		t.Errorf("DELETE.IntValue() = %d, want 4", DELETE.IntValue())
	}
}
