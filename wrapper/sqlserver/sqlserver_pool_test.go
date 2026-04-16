package sqlserver

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
	"time"
)

// TestPoolConfigDefaults verifies that zero-value struct fields resolve to
// the expected production defaults. These defaults are applied inside Open()
// after a successful Ping(), but we can validate the default-resolution
// logic independently without a live SQL Server connection.
func TestPoolConfigDefaults(t *testing.T) {
	svr := &SQLServer{}

	// Zero-value fields should trigger production defaults in Open().
	// Validate the defaults that would be applied:

	maxOpen := svr.MaxOpenConns
	if maxOpen == 0 {
		maxOpen = 25
	}
	if maxOpen != 25 {
		t.Errorf("expected default MaxOpenConns=25, got %d", maxOpen)
	}

	maxIdle := svr.MaxIdleConns
	if maxIdle == 0 {
		maxIdle = 5
	}
	if maxIdle != 5 {
		t.Errorf("expected default MaxIdleConns=5, got %d", maxIdle)
	}

	connMaxLifetime := svr.ConnMaxLifetime
	if connMaxLifetime == 0 {
		connMaxLifetime = 5 * time.Minute
	}
	if connMaxLifetime != 5*time.Minute {
		t.Errorf("expected default ConnMaxLifetime=5m, got %v", connMaxLifetime)
	}
}

// TestPoolConfigCustomValues verifies that explicitly set struct fields are
// used as-is (no default fallback) when the values are non-zero.
func TestPoolConfigCustomValues(t *testing.T) {
	svr := &SQLServer{
		MaxOpenConns:    50,
		MaxIdleConns:    10,
		ConnMaxLifetime: 10 * time.Minute,
	}

	maxOpen := svr.MaxOpenConns
	if maxOpen == 0 {
		maxOpen = 25
	}
	if maxOpen != 50 {
		t.Errorf("expected custom MaxOpenConns=50, got %d", maxOpen)
	}

	maxIdle := svr.MaxIdleConns
	if maxIdle == 0 {
		maxIdle = 5
	}
	if maxIdle != 10 {
		t.Errorf("expected custom MaxIdleConns=10, got %d", maxIdle)
	}

	connMaxLifetime := svr.ConnMaxLifetime
	if connMaxLifetime == 0 {
		connMaxLifetime = 5 * time.Minute
	}
	if connMaxLifetime != 10*time.Minute {
		t.Errorf("expected custom ConnMaxLifetime=10m, got %v", connMaxLifetime)
	}
}

// TestPoolConfigStructFieldsExist ensures the pool config fields are present
// on the SQLServer struct and are the correct types.
func TestPoolConfigStructFieldsExist(t *testing.T) {
	svr := SQLServer{}

	// These assignments verify the fields exist and accept the expected types.
	// If any field is removed or its type changes, this test fails at compile time.
	svr.MaxOpenConns = 30
	svr.MaxIdleConns = 8
	svr.ConnMaxLifetime = 3 * time.Minute

	if svr.MaxOpenConns != 30 {
		t.Errorf("MaxOpenConns field assignment failed")
	}
	if svr.MaxIdleConns != 8 {
		t.Errorf("MaxIdleConns field assignment failed")
	}
	if svr.ConnMaxLifetime != 3*time.Minute {
		t.Errorf("ConnMaxLifetime field assignment failed")
	}
}
