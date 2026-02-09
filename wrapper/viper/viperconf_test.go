package viper

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

import (
	"testing"
)

// TestNilReceiverChecks verifies that all public methods handle nil receivers gracefully
func TestNilReceiverChecks(t *testing.T) {
	var v *ViperConf = nil

	// Test ensureMu - should not panic
	v.ensureMu()

	// Test getClient - should return nil
	if client := v.getClient(); client != nil {
		t.Errorf("getClient() on nil receiver should return nil, got %v", client)
	}

	// Test setClient - should not panic
	v.setClient(nil)

	// Test Init - should return error
	if ok, err := v.Init(); ok != false || err == nil {
		t.Errorf("Init() on nil receiver should return false and error, got ok=%v, err=%v", ok, err)
	}

	// Test WatchConfig - should not panic
	v.WatchConfig()

	// Test ConfigFileUsed - should return empty string
	if file := v.ConfigFileUsed(); file != "" {
		t.Errorf("ConfigFileUsed() on nil receiver should return empty string, got %v", file)
	}

	// Test Default - should return nil
	if result := v.Default("key", "value"); result != nil {
		t.Errorf("Default() on nil receiver should return nil, got %v", result)
	}

	// Test Unmarshal - should return error
	if err := v.Unmarshal(nil); err == nil {
		t.Errorf("Unmarshal() on nil receiver should return error, got nil")
	}

	// Test SubConf - should return nil
	if result := v.SubConf("key"); result != nil {
		t.Errorf("SubConf() on nil receiver should return nil, got %v", result)
	}

	// Test Set - should return nil
	if result := v.Set("key", "value"); result != nil {
		t.Errorf("Set() on nil receiver should return nil, got %v", result)
	}

	// Test Save - should return error
	if err := v.Save(); err == nil {
		t.Errorf("Save() on nil receiver should return error, got nil")
	}

	// Test Alias - should return nil
	if result := v.Alias("key", "alias"); result != nil {
		t.Errorf("Alias() on nil receiver should return nil, got %v", result)
	}

	// Test getter methods
	if v.IsDefined("key") != false {
		t.Error("IsDefined() on nil receiver should return false")
	}

	if v.IsSet("key") != false {
		t.Error("IsSet() on nil receiver should return false")
	}

	if v.Size("key") != 0 {
		t.Error("Size() on nil receiver should return 0")
	}

	if v.Get("key") != nil {
		t.Error("Get() on nil receiver should return nil")
	}

	if v.GetInt("key") != 0 {
		t.Error("GetInt() on nil receiver should return 0")
	}

	if v.GetIntSlice("key") != nil {
		t.Error("GetIntSlice() on nil receiver should return nil")
	}

	if v.GetInt64("key") != 0 {
		t.Error("GetInt64() on nil receiver should return 0")
	}

	if v.GetFloat64("key") != 0.00 {
		t.Error("GetFloat64() on nil receiver should return 0.00")
	}

	if v.GetBool("key") != false {
		t.Error("GetBool() on nil receiver should return false")
	}

	if !v.GetTime("key").IsZero() {
		t.Error("GetTime() on nil receiver should return zero time")
	}

	if v.GetDuration("key") != 0 {
		t.Error("GetDuration() on nil receiver should return 0")
	}

	if v.GetString("key") != "" {
		t.Error("GetString() on nil receiver should return empty string")
	}

	if v.GetStringSlice("key") != nil {
		t.Error("GetStringSlice() on nil receiver should return nil")
	}

	if v.GetStringMapInterface("key") != nil {
		t.Error("GetStringMapInterface() on nil receiver should return nil")
	}

	if v.GetStringMapString("key") != nil {
		t.Error("GetStringMapString() on nil receiver should return nil")
	}

	if v.GetStringMapStringSlice("key") != nil {
		t.Error("GetStringMapStringSlice() on nil receiver should return nil")
	}

	if v.AllKeys() != nil {
		t.Error("AllKeys() on nil receiver should return nil")
	}

	if v.AllSettings() != nil {
		t.Error("AllSettings() on nil receiver should return nil")
	}
}
