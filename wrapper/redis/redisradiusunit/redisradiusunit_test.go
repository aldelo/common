package redisradiusunit

import "testing"

func TestValueSliceCompleteness(t *testing.T) {
	values := UNKNOWN.ValueSlice()
	if got := len(values); got != 5 {
		t.Fatalf("ValueSlice length = %d, want 5", got)
	}
}

func TestStringRoundTrip(t *testing.T) {
	for _, v := range UNKNOWN.ValueSlice() {
		name := v.String()
		got, err := v.ParseByName(name)
		if err != nil {
			t.Errorf("ParseByName(%q) error: %v", name, err)
		}
		if got != v {
			t.Errorf("ParseByName(%q) = %d, want %d", name, got, v)
		}
	}
}

func TestKeyRoundTrip(t *testing.T) {
	for _, v := range UNKNOWN.ValueSlice() {
		key := v.Key()
		got, err := v.ParseByKey(key)
		if err != nil {
			t.Errorf("ParseByKey(%q) error: %v", key, err)
		}
		if got != v {
			t.Errorf("ParseByKey(%q) = %d, want %d", key, got, v)
		}
	}
}

func TestValid(t *testing.T) {
	for _, v := range UNKNOWN.ValueSlice() {
		if !v.Valid() {
			t.Errorf("%d (%s) should be Valid", v, v.String())
		}
	}
}

func TestParseByNameInvalid(t *testing.T) {
	_, err := UNKNOWN.ParseByName("BOGUS_VALUE")
	if err == nil {
		t.Error("ParseByName(BOGUS_VALUE) should return error")
	}
}

func TestParseByKeyInvalid(t *testing.T) {
	_, err := UNKNOWN.ParseByKey("BOGUS_KEY")
	if err == nil {
		t.Error("ParseByKey(BOGUS_KEY) should return error")
	}
}
