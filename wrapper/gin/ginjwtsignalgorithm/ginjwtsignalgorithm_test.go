package ginjwtsignalgorithm

import "testing"

// allValues enumerates every defined GinJwtSignAlgorithm constant.
var allValues = []struct {
	val  GinJwtSignAlgorithm
	name string
	key  string
	iota int
}{
	{UNKNOWN, "UNKNOWN", "UNKNOWN", 0},
	{HS256, "HS256", "HS256", 1},
	{HS384, "HS384", "HS384", 2},
	{HS512, "HS512", "HS512", 3},
	{RS256, "RS256", "RS256", 4},
	{RS384, "RS384", "RS384", 5},
	{RS512, "RS512", "RS512", 6},
}

func TestValueSliceCompleteness(t *testing.T) {
	vs := UNKNOWN.ValueSlice()
	if len(vs) != len(allValues) {
		t.Fatalf("ValueSlice length = %d, want %d", len(vs), len(allValues))
	}
	m := make(map[GinJwtSignAlgorithm]bool, len(vs))
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
	invalid := GinJwtSignAlgorithm(-1)
	if invalid.Valid() {
		t.Error("GinJwtSignAlgorithm(-1) should not be Valid")
	}
}

func TestParseByNameInvalid(t *testing.T) {
	_, err := UNKNOWN.ParseByName("ES256")
	if err == nil {
		t.Error("ParseByName(\"ES256\") should return error")
	}
}

func TestParseByKeyInvalid(t *testing.T) {
	_, err := UNKNOWN.ParseByKey("ES256")
	if err == nil {
		t.Error("ParseByKey(\"ES256\") should return error")
	}
}

func TestIntValueSpotCheck(t *testing.T) {
	if RS512.IntValue() != 6 {
		t.Errorf("RS512.IntValue() = %d, want 6", RS512.IntValue())
	}
}
