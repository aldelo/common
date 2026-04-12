package awsregion

import (
	"testing"
)

// TestValueSliceCompleteness verifies that ValueSlice returns all 9 defined enum values
// (UNKNOWN + 8 AWS regions).
func TestValueSliceCompleteness(t *testing.T) {
	values := UNKNOWN.ValueSlice()

	expectedCount := 9 // UNKNOWN + 8 regions
	if len(values) != expectedCount {
		t.Fatalf("ValueSlice() returned %d values, expected %d", len(values), expectedCount)
	}

	// Every defined constant must appear in the slice
	expected := []AWSRegion{
		UNKNOWN,
		AWS_us_west_2_oregon,
		AWS_us_east_1_nvirginia,
		AWS_eu_west_2_london,
		AWS_eu_central_1_frankfurt,
		AWS_ap_southeast_1_singapore,
		AWS_ap_east_1_hongkong,
		AWS_ap_northeast_1_tokyo,
		AWS_ap_southeast_2_sydney,
	}

	valueSet := make(map[AWSRegion]bool, len(values))
	for _, v := range values {
		valueSet[v] = true
	}

	for _, e := range expected {
		if !valueSet[e] {
			t.Errorf("ValueSlice() missing expected value %d (%s)", int(e), e.String())
		}
	}
}

// TestStringParseByNameRoundTrip verifies that String() and ParseByName() are inverses
// for all defined enum values.
func TestStringParseByNameRoundTrip(t *testing.T) {
	values := UNKNOWN.ValueSlice()

	for _, v := range values {
		name := v.String()
		if name == "" {
			t.Errorf("String() returned empty for value %d", int(v))
			continue
		}

		parsed, err := v.ParseByName(name)
		if err != nil {
			t.Errorf("ParseByName(%q) returned error: %v", name, err)
			continue
		}

		if parsed != v {
			t.Errorf("Round-trip failed: started with %d, String()=%q, ParseByName returned %d", int(v), name, int(parsed))
		}
	}
}

// TestKeyParseByKeyRoundTrip verifies that Key() and ParseByKey() are inverses
// for all defined enum values.
func TestKeyParseByKeyRoundTrip(t *testing.T) {
	values := UNKNOWN.ValueSlice()

	for _, v := range values {
		key := v.Key()
		if key == "" {
			t.Errorf("Key() returned empty for value %d (%s)", int(v), v.String())
			continue
		}

		parsed, err := v.ParseByKey(key)
		if err != nil {
			t.Errorf("ParseByKey(%q) returned error: %v", key, err)
			continue
		}

		if parsed != v {
			t.Errorf("Round-trip failed: started with %d, Key()=%q, ParseByKey returned %d", int(v), key, int(parsed))
		}
	}
}

// TestValidForAllDefinedValues verifies that Valid() returns true for every value
// in the enum slice.
func TestValidForAllDefinedValues(t *testing.T) {
	values := UNKNOWN.ValueSlice()

	for _, v := range values {
		if !v.Valid() {
			t.Errorf("Valid() returned false for defined value %d (%s)", int(v), v.String())
		}
	}
}

// TestValidForInvalidValue verifies that Valid() returns false for values
// outside the defined enum range.
func TestValidForInvalidValue(t *testing.T) {
	invalid := AWSRegion(999)
	if invalid.Valid() {
		t.Error("Valid() returned true for undefined value 999")
	}

	negative := AWSRegion(-1)
	if negative.Valid() {
		t.Error("Valid() returned true for negative value -1")
	}
}

// TestAWSRegionKeyMapping verifies that each region enum maps to the correct
// AWS region string identifier.
func TestAWSRegionKeyMapping(t *testing.T) {
	tests := []struct {
		region      AWSRegion
		expectedKey string
	}{
		{UNKNOWN, "UNKNOWN"},
		{AWS_us_west_2_oregon, "us-west-2"},
		{AWS_us_east_1_nvirginia, "us-east-1"},
		{AWS_eu_west_2_london, "eu-west-2"},
		{AWS_eu_central_1_frankfurt, "eu-central-1"},
		{AWS_ap_southeast_1_singapore, "ap-southeast-1"},
		{AWS_ap_east_1_hongkong, "ap-east-1"},
		{AWS_ap_northeast_1_tokyo, "ap-northeast-1"},
		{AWS_ap_southeast_2_sydney, "ap-southeast-2"},
	}

	for _, tc := range tests {
		t.Run(tc.expectedKey, func(t *testing.T) {
			got := tc.region.Key()
			if got != tc.expectedKey {
				t.Errorf("Key() = %q, expected %q", got, tc.expectedKey)
			}
		})
	}
}

// TestGetAwsRegion verifies the GetAwsRegion function maps region strings
// to the correct enum values.
func TestGetAwsRegion(t *testing.T) {
	tests := []struct {
		input    string
		expected AWSRegion
	}{
		{"us-west-2", AWS_us_west_2_oregon},
		{"us-east-1", AWS_us_east_1_nvirginia},
		{"eu-west-2", AWS_eu_west_2_london},
		{"eu-central-1", AWS_eu_central_1_frankfurt},
		{"ap-southeast-1", AWS_ap_southeast_1_singapore},
		{"ap-east-1", AWS_ap_east_1_hongkong},
		{"ap-northeast-1", AWS_ap_northeast_1_tokyo},
		{"ap-southeast-2", AWS_ap_southeast_2_sydney},
		// Case insensitivity
		{"US-EAST-1", AWS_us_east_1_nvirginia},
		{"Us-West-2", AWS_us_west_2_oregon},
		// Unknown region
		{"", UNKNOWN},
		{"invalid-region", UNKNOWN},
		{"us-east-2", UNKNOWN},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := GetAwsRegion(tc.input)
			if got != tc.expected {
				t.Errorf("GetAwsRegion(%q) = %d (%s), expected %d (%s)",
					tc.input, int(got), got.String(), int(tc.expected), tc.expected.String())
			}
		})
	}
}

// TestStringForUndefinedValue verifies that String() returns empty string
// for values not in the enum.
func TestStringForUndefinedValue(t *testing.T) {
	undefined := AWSRegion(100)
	if s := undefined.String(); s != "" {
		t.Errorf("String() for undefined value 100 returned %q, expected empty string", s)
	}
}

// TestKeyForUndefinedValue verifies that Key() returns empty string
// for values not in the enum.
func TestKeyForUndefinedValue(t *testing.T) {
	undefined := AWSRegion(100)
	if k := undefined.Key(); k != "" {
		t.Errorf("Key() for undefined value 100 returned %q, expected empty string", k)
	}
}

// TestParseByNameError verifies that ParseByName returns an error for
// an unrecognized name.
func TestParseByNameError(t *testing.T) {
	_, err := UNKNOWN.ParseByName("nonexistent")
	if err == nil {
		t.Error("ParseByName(\"nonexistent\") should return error, got nil")
	}
}

// TestParseByKeyError verifies that ParseByKey returns an error for
// an unrecognized key.
func TestParseByKeyError(t *testing.T) {
	_, err := UNKNOWN.ParseByKey("nonexistent-region")
	if err == nil {
		t.Error("ParseByKey(\"nonexistent-region\") should return error, got nil")
	}
}

// TestIntValueAndIntString verifies IntValue and IntString for all enum values.
func TestIntValueAndIntString(t *testing.T) {
	tests := []struct {
		region    AWSRegion
		intVal    int
		intStr    string
	}{
		{UNKNOWN, 0, "0"},
		{AWS_us_west_2_oregon, 1, "1"},
		{AWS_us_east_1_nvirginia, 2, "2"},
		{AWS_eu_west_2_london, 3, "3"},
		{AWS_eu_central_1_frankfurt, 4, "4"},
		{AWS_ap_southeast_1_singapore, 5, "5"},
		{AWS_ap_east_1_hongkong, 6, "6"},
		{AWS_ap_northeast_1_tokyo, 7, "7"},
		{AWS_ap_southeast_2_sydney, 8, "8"},
	}

	for _, tc := range tests {
		if got := tc.region.IntValue(); got != tc.intVal {
			t.Errorf("%s.IntValue() = %d, expected %d", tc.region.String(), got, tc.intVal)
		}
		if got := tc.region.IntString(); got != tc.intStr {
			t.Errorf("%s.IntString() = %q, expected %q", tc.region.String(), got, tc.intStr)
		}
	}
}

// TestCaptionAndDescription verifies Caption() and Description() return
// non-empty strings for all defined values.
func TestCaptionAndDescription(t *testing.T) {
	values := UNKNOWN.ValueSlice()

	for _, v := range values {
		caption := v.Caption()
		if caption == "" {
			t.Errorf("Caption() returned empty for value %d (%s)", int(v), v.String())
		}

		desc := v.Description()
		if desc == "" {
			t.Errorf("Description() returned empty for value %d (%s)", int(v), v.String())
		}
	}
}

// TestNameMapKeyMapCaptionMapDescriptionMap verifies the map accessors return
// maps with the expected number of entries.
func TestNameMapKeyMapCaptionMapDescriptionMap(t *testing.T) {
	expectedCount := 9

	if got := len(UNKNOWN.NameMap()); got != expectedCount {
		t.Errorf("NameMap() has %d entries, expected %d", got, expectedCount)
	}

	if got := len(UNKNOWN.KeyMap()); got != expectedCount {
		t.Errorf("KeyMap() has %d entries, expected %d", got, expectedCount)
	}

	if got := len(UNKNOWN.CaptionMap()); got != expectedCount {
		t.Errorf("CaptionMap() has %d entries, expected %d", got, expectedCount)
	}

	if got := len(UNKNOWN.DescriptionMap()); got != expectedCount {
		t.Errorf("DescriptionMap() has %d entries, expected %d", got, expectedCount)
	}
}
