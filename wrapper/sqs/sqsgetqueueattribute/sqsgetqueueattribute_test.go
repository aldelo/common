package sqsgetqueueattribute

import "testing"

func TestValueSliceCompleteness(t *testing.T) {
	vs := UNKNOWN.ValueSlice()
	if len(vs) != 21 {
		t.Fatalf("ValueSlice length = %d; want 21", len(vs))
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
			t.Errorf("ParseByName(%q) = %d; want %d", name, got, v)
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
			t.Errorf("ParseByKey(%q) = %d; want %d", key, got, v)
		}
	}
}

func TestValid(t *testing.T) {
	for _, v := range UNKNOWN.ValueSlice() {
		if !v.Valid() {
			t.Errorf("%d.Valid() = false; want true", v)
		}
	}
}

func TestParseByNameInvalid(t *testing.T) {
	_, err := UNKNOWN.ParseByName("DOES_NOT_EXIST")
	if err == nil {
		t.Error("ParseByName(DOES_NOT_EXIST) expected error; got nil")
	}
}

func TestParseByKeyInvalid(t *testing.T) {
	_, err := UNKNOWN.ParseByKey("DOES_NOT_EXIST")
	if err == nil {
		t.Error("ParseByKey(DOES_NOT_EXIST) expected error; got nil")
	}
}
