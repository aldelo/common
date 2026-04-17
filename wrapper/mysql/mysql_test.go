package mysql

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
	"fmt"
	"strings"
	"testing"
)

// =====================================================================
// QueryBuilder Tests
// =====================================================================

func TestQueryBuilder_Set_SinglePart(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT * FROM users")
	qb.Build()

	got := qb.SQL()
	want := "SELECT * FROM users"

	if got != want {
		t.Errorf("SQL() = %q, want %q", got, want)
	}
}

func TestQueryBuilder_Set_MultipleParts(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT * FROM users")
	qb.Set(" WHERE id = ?")
	qb.Set(" AND active = 1")
	qb.Build()

	got := qb.SQL()
	want := "SELECT * FROM users WHERE id = ? AND active = 1"

	if got != want {
		t.Errorf("SQL() = %q, want %q", got, want)
	}
}

func TestQueryBuilder_Set_EmptyString(t *testing.T) {
	var qb QueryBuilder
	qb.Set("")
	qb.Build()

	got := qb.SQL()
	if got != "" {
		t.Errorf("SQL() = %q, want empty string", got)
	}
}

func TestQueryBuilder_EmptyBuilder_SQL(t *testing.T) {
	var qb QueryBuilder
	got := qb.SQL()
	if got != "" {
		t.Errorf("SQL() on empty builder = %q, want empty string", got)
	}
}

func TestQueryBuilder_EmptyBuilder_ParamsMap(t *testing.T) {
	var qb QueryBuilder
	got := qb.ParamsMap()
	if got != nil {
		t.Errorf("ParamsMap() on empty builder = %v, want nil", got)
	}
}

func TestQueryBuilder_EmptyBuilder_ParamsSlice(t *testing.T) {
	var qb QueryBuilder
	got := qb.ParamsSlice()
	if got != nil {
		t.Errorf("ParamsSlice() on empty builder = %v, want nil", got)
	}
}

func TestQueryBuilder_SQL_WithoutBuild(t *testing.T) {
	// SQL() has a fallback: if output is empty, it reads from buffer directly
	var qb QueryBuilder
	qb.Set("SELECT 1")

	got := qb.SQL()
	want := "SELECT 1"

	if got != want {
		t.Errorf("SQL() without Build() = %q, want %q", got, want)
	}
}

func TestQueryBuilder_Named_Table(t *testing.T) {
	tests := []struct {
		name       string
		paramName  string
		paramValue interface{}
		wantStored bool
	}{
		{
			name:       "string value",
			paramName:  "username",
			paramValue: "john",
			wantStored: true,
		},
		{
			name:       "integer value",
			paramName:  "user_id",
			paramValue: 42,
			wantStored: true,
		},
		{
			name:       "nil value",
			paramName:  "nullable",
			paramValue: nil,
			wantStored: true,
		},
		{
			name:       "empty param name ignored",
			paramName:  "",
			paramValue: "should not store",
			wantStored: false,
		},
		{
			name:       "whitespace-only param name ignored",
			paramName:  "   ",
			paramValue: "should not store",
			wantStored: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var qb QueryBuilder
			qb.Named(tt.paramName, tt.paramValue)

			m := qb.ParamsMap()
			if tt.wantStored {
				if m == nil {
					t.Fatal("ParamsMap() returned nil, expected map with entry")
				}
				val, ok := m[tt.paramName]
				if !ok {
					t.Errorf("ParamsMap() missing key %q", tt.paramName)
				}
				if val != tt.paramValue {
					t.Errorf("ParamsMap()[%q] = %v, want %v", tt.paramName, val, tt.paramValue)
				}
			} else {
				if len(m) > 0 {
					t.Errorf("ParamsMap() should be empty for ignored param name, got %v", m)
				}
			}
		})
	}
}

func TestQueryBuilder_Named_OverwriteExisting(t *testing.T) {
	var qb QueryBuilder
	qb.Named("key", "original")
	qb.Named("key", "updated")

	m := qb.ParamsMap()
	if m["key"] != "updated" {
		t.Errorf("Named() did not overwrite; got %v, want %q", m["key"], "updated")
	}
	if len(m) != 1 {
		t.Errorf("ParamsMap() length = %d, want 1 after overwrite", len(m))
	}
}

func TestQueryBuilder_Named_MultipleKeys(t *testing.T) {
	var qb QueryBuilder
	qb.Named("a", 1)
	qb.Named("b", 2)
	qb.Named("c", 3)

	m := qb.ParamsMap()
	if len(m) != 3 {
		t.Errorf("ParamsMap() length = %d, want 3", len(m))
	}
	for _, key := range []string{"a", "b", "c"} {
		if _, ok := m[key]; !ok {
			t.Errorf("ParamsMap() missing key %q", key)
		}
	}
}

func TestQueryBuilder_Ordinal_SingleParam(t *testing.T) {
	var qb QueryBuilder
	qb.Ordinal("value1")

	s := qb.ParamsSlice()
	if len(s) != 1 {
		t.Fatalf("ParamsSlice() length = %d, want 1", len(s))
	}
	if s[0] != "value1" {
		t.Errorf("ParamsSlice()[0] = %v, want %q", s[0], "value1")
	}
}

func TestQueryBuilder_Ordinal_MultipleParams(t *testing.T) {
	var qb QueryBuilder
	qb.Ordinal("first")
	qb.Ordinal(42)
	qb.Ordinal(nil)
	qb.Ordinal(true)

	s := qb.ParamsSlice()
	if len(s) != 4 {
		t.Fatalf("ParamsSlice() length = %d, want 4", len(s))
	}

	expected := []interface{}{"first", 42, nil, true}
	for i, want := range expected {
		if s[i] != want {
			t.Errorf("ParamsSlice()[%d] = %v, want %v", i, s[i], want)
		}
	}
}

func TestQueryBuilder_Ordinal_PreservesOrder(t *testing.T) {
	var qb QueryBuilder
	for i := 0; i < 10; i++ {
		qb.Ordinal(i)
	}

	s := qb.ParamsSlice()
	if len(s) != 10 {
		t.Fatalf("ParamsSlice() length = %d, want 10", len(s))
	}
	for i := 0; i < 10; i++ {
		if s[i] != i {
			t.Errorf("ParamsSlice()[%d] = %v, want %d", i, s[i], i)
		}
	}
}

func TestQueryBuilder_MixedNamedAndOrdinal(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT * FROM orders WHERE status = :status AND amount > ?")
	qb.Named("status", "active")
	qb.Ordinal(100)
	qb.Build()

	sql := qb.SQL()
	if sql != "SELECT * FROM orders WHERE status = :status AND amount > ?" {
		t.Errorf("SQL() = %q, unexpected value", sql)
	}

	m := qb.ParamsMap()
	if m["status"] != "active" {
		t.Errorf("named param 'status' = %v, want %q", m["status"], "active")
	}

	s := qb.ParamsSlice()
	if len(s) != 1 || s[0] != 100 {
		t.Errorf("ordinal params = %v, want [100]", s)
	}
}

func TestQueryBuilder_ClearAll(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT 1")
	qb.Named("key", "value")
	qb.Ordinal(42)
	qb.Build()

	// Verify non-empty state
	if qb.SQL() == "" {
		t.Fatal("precondition: SQL() should be non-empty before ClearAll")
	}

	qb.ClearAll()

	if sql := qb.SQL(); sql != "" {
		t.Errorf("after ClearAll, SQL() = %q, want empty", sql)
	}
	if m := qb.ParamsMap(); len(m) != 0 {
		t.Errorf("after ClearAll, ParamsMap() = %v, want empty", m)
	}
	if s := qb.ParamsSlice(); len(s) != 0 {
		t.Errorf("after ClearAll, ParamsSlice() = %v, want empty", s)
	}
}

func TestQueryBuilder_ClearAll_ThenReuse(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT 1")
	qb.Named("old", "data")
	qb.Ordinal("old_ordinal")
	qb.Build()

	qb.ClearAll()

	qb.Set("SELECT 2")
	qb.Named("new", "data")
	qb.Ordinal("new_ordinal")
	qb.Build()

	if sql := qb.SQL(); sql != "SELECT 2" {
		t.Errorf("after ClearAll + rebuild, SQL() = %q, want %q", sql, "SELECT 2")
	}
	if m := qb.ParamsMap(); m["new"] != "data" {
		t.Errorf("after ClearAll + rebuild, ParamsMap() = %v", m)
	}
	if _, ok := qb.ParamsMap()["old"]; ok {
		t.Error("old named param should not exist after ClearAll")
	}
	if s := qb.ParamsSlice(); len(s) != 1 || s[0] != "new_ordinal" {
		t.Errorf("after ClearAll + rebuild, ParamsSlice() = %v", s)
	}
}

func TestQueryBuilder_ClearSQL(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT 1")
	qb.Named("key", "value")
	qb.Ordinal(42)
	qb.Build()

	qb.ClearSQL()

	// SQL should be cleared
	if sql := qb.SQL(); sql != "" {
		t.Errorf("after ClearSQL, SQL() = %q, want empty", sql)
	}

	// Params should remain intact
	if m := qb.ParamsMap(); m["key"] != "value" {
		t.Errorf("after ClearSQL, ParamsMap() should retain params, got %v", m)
	}
	if s := qb.ParamsSlice(); len(s) != 1 || s[0] != 42 {
		t.Errorf("after ClearSQL, ParamsSlice() should retain params, got %v", s)
	}
}

func TestQueryBuilder_ClearSQL_ThenRebuild(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT old")
	qb.Build()

	qb.ClearSQL()
	qb.Set("SELECT new")
	qb.Build()

	if sql := qb.SQL(); sql != "SELECT new" {
		t.Errorf("after ClearSQL + rebuild, SQL() = %q, want %q", sql, "SELECT new")
	}
}

func TestQueryBuilder_ClearParams(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT 1")
	qb.Named("key", "value")
	qb.Ordinal(42)
	qb.Build()

	qb.ClearParams()

	// SQL should remain intact
	if sql := qb.SQL(); sql != "SELECT 1" {
		t.Errorf("after ClearParams, SQL() = %q, want %q", sql, "SELECT 1")
	}

	// Params should be cleared
	if m := qb.ParamsMap(); len(m) != 0 {
		t.Errorf("after ClearParams, ParamsMap() = %v, want empty", m)
	}
	if s := qb.ParamsSlice(); len(s) != 0 {
		t.Errorf("after ClearParams, ParamsSlice() = %v, want empty", s)
	}
}

func TestQueryBuilder_ClearParams_ThenReAddParams(t *testing.T) {
	var qb QueryBuilder
	qb.Named("old", "data")
	qb.Ordinal("old_val")

	qb.ClearParams()

	qb.Named("new", "data")
	qb.Ordinal("new_val")

	m := qb.ParamsMap()
	if _, ok := m["old"]; ok {
		t.Error("old named param should not exist after ClearParams")
	}
	if m["new"] != "data" {
		t.Errorf("ParamsMap()[\"new\"] = %v, want %q", m["new"], "data")
	}

	s := qb.ParamsSlice()
	if len(s) != 1 || s[0] != "new_val" {
		t.Errorf("ParamsSlice() = %v, want [new_val]", s)
	}
}

func TestQueryBuilder_Build_FreezesOutput(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT 1")
	qb.Build()

	// After Build(), appending more SQL to the buffer should not change output
	// until Build() is called again (because SQL() returns output if non-empty).
	qb.Set(" EXTRA")

	got := qb.SQL()
	want := "SELECT 1"
	if got != want {
		t.Errorf("SQL() after Build + more Set = %q, want %q (output frozen after Build)", got, want)
	}

	// After re-building, the new content should appear
	qb.Build()
	got = qb.SQL()
	want = "SELECT 1 EXTRA"
	if got != want {
		t.Errorf("SQL() after second Build = %q, want %q", got, want)
	}
}

func TestQueryBuilder_LargeQuery(t *testing.T) {
	var qb QueryBuilder
	qb.Set("SELECT ")

	columns := make([]string, 100)
	for i := range columns {
		columns[i] = "col" + strings.Repeat("x", 10)
	}
	qb.Set(strings.Join(columns, ", "))
	qb.Set(" FROM large_table")
	qb.Build()

	sql := qb.SQL()
	if !strings.HasPrefix(sql, "SELECT ") {
		t.Error("large query should start with SELECT")
	}
	if !strings.HasSuffix(sql, " FROM large_table") {
		t.Error("large query should end with FROM large_table")
	}
}

// =====================================================================
// GetDsn Tests
// =====================================================================

func TestGetDsn_AllFieldsPopulated(t *testing.T) {
	svr := &MySql{
		UserName:       "admin",
		Password:       "secret",
		Host:           "db.example.com",
		Port:           3306,
		Database:       "testdb",
		Charset:        "utf8",
		Collation:      "utf8_general_ci",
		ConnectTimeout: "30s",
		ReadTimeout:    "10s",
		WriteTimeout:   "15s",
		RejectReadOnly: true,
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}

	// Verify the full DSN format
	want := "admin:secret@(db.example.com:3306)/testdb?charset=utf8&collation=utf8_general_ci&parseTime=true&timeout=30s&readTimeout=10s&writeTimeout=15s&rejectReadOnly=true"
	if dsn != want {
		t.Errorf("GetDsn() = %q, want %q", dsn, want)
	}
}

func TestGetDsn_MinimalRequired(t *testing.T) {
	svr := &MySql{
		UserName: "user",
		Password: "pass",
		Host:     "localhost",
		Database: "mydb",
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}

	// Port omitted (0), defaults for charset/collation, and production-safe
	// 30s defaults applied to readTimeout/writeTimeout (P3-N1, 2026-04-17).
	// ConnectTimeout remains opt-in so its key is absent.
	want := "user:pass@(localhost)/mydb?charset=utf8mb4&collation=utf8mb4_general_ci&parseTime=true&readTimeout=30s&writeTimeout=30s"
	if dsn != want {
		t.Errorf("GetDsn() = %q, want %q", dsn, want)
	}
}

func TestGetDsn_RequiredFields_Table(t *testing.T) {
	tests := []struct {
		name    string
		svr     *MySql
		wantErr string
	}{
		{
			name: "missing username",
			svr: &MySql{
				Password: "pass",
				Host:     "localhost",
				Database: "db",
			},
			wantErr: "User Name is Required",
		},
		{
			name: "missing password",
			svr: &MySql{
				UserName: "user",
				Host:     "localhost",
				Database: "db",
			},
			wantErr: "Password is Required",
		},
		{
			name: "missing host",
			svr: &MySql{
				UserName: "user",
				Password: "pass",
				Database: "db",
			},
			wantErr: "MySQL Host Address is Required",
		},
		{
			name: "missing database",
			svr: &MySql{
				UserName: "user",
				Password: "pass",
				Host:     "localhost",
			},
			wantErr: "MySQL Database Name is Required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn, err := tt.svr.GetDsn()
			if err == nil {
				t.Fatalf("GetDsn() = %q, want error containing %q", dsn, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("GetDsn() error = %q, want error containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestGetDsn_PortVariations(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		wantPart string // substring that should appear in DSN
	}{
		{
			name:     "zero port omitted",
			port:     0,
			wantPart: "@(localhost)/",
		},
		{
			name:     "negative port omitted",
			port:     -1,
			wantPart: "@(localhost)/",
		},
		{
			name:     "standard port",
			port:     3306,
			wantPart: "@(localhost:3306)/",
		},
		{
			name:     "custom port",
			port:     3307,
			wantPart: "@(localhost:3307)/",
		},
		{
			name:     "high port number",
			port:     65535,
			wantPart: "@(localhost:65535)/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svr := &MySql{
				UserName: "user",
				Password: "pass",
				Host:     "localhost",
				Port:     tt.port,
				Database: "db",
			}

			dsn, err := svr.GetDsn()
			if err != nil {
				t.Fatalf("GetDsn() error = %v", err)
			}
			if !strings.Contains(dsn, tt.wantPart) {
				t.Errorf("GetDsn() = %q, want substring %q", dsn, tt.wantPart)
			}
		})
	}
}

func TestGetDsn_CharsetDefault(t *testing.T) {
	svr := &MySql{
		UserName: "user",
		Password: "pass",
		Host:     "localhost",
		Database: "db",
		Charset:  "", // empty -> default utf8mb4
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}
	if !strings.Contains(dsn, "charset=utf8mb4") {
		t.Errorf("GetDsn() = %q, want default charset=utf8mb4", dsn)
	}
}

func TestGetDsn_CharsetCustom(t *testing.T) {
	svr := &MySql{
		UserName: "user",
		Password: "pass",
		Host:     "localhost",
		Database: "db",
		Charset:  "utf8",
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}
	if !strings.Contains(dsn, "charset=utf8&") {
		t.Errorf("GetDsn() = %q, want charset=utf8", dsn)
	}
}

func TestGetDsn_CollationDefault(t *testing.T) {
	svr := &MySql{
		UserName: "user",
		Password: "pass",
		Host:     "localhost",
		Database: "db",
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}
	if !strings.Contains(dsn, "collation=utf8mb4_general_ci") {
		t.Errorf("GetDsn() = %q, want default collation=utf8mb4_general_ci", dsn)
	}
}

func TestGetDsn_CollationCustom(t *testing.T) {
	svr := &MySql{
		UserName:  "user",
		Password:  "pass",
		Host:      "localhost",
		Database:  "db",
		Collation: "utf8_unicode_ci",
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}
	if !strings.Contains(dsn, "collation=utf8_unicode_ci") {
		t.Errorf("GetDsn() = %q, want collation=utf8_unicode_ci", dsn)
	}
}

func TestGetDsn_ParseTimeAlwaysTrue(t *testing.T) {
	svr := &MySql{
		UserName: "user",
		Password: "pass",
		Host:     "localhost",
		Database: "db",
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}
	if !strings.Contains(dsn, "parseTime=true") {
		t.Errorf("GetDsn() = %q, want parseTime=true always present", dsn)
	}
}

// TestGetDsn_TimeoutOptions verifies DSN timeout serialization.
// Post-P3-N1 (2026-04-17): readTimeout and writeTimeout ALWAYS appear in the
// DSN — either the operator-supplied value or the production-safe 30s default.
// Only ConnectTimeout remains opt-in (empty → no timeout= key in DSN) because
// connect-time hangs are covered by the database/sql driver's DialTimeout and
// a blocking connect is typically caught by upstream network monitoring long
// before an in-flight read/write hang would be.
func TestGetDsn_TimeoutOptions(t *testing.T) {
	tests := []struct {
		name            string
		connect         string
		read            string
		write           string
		wantTimeout     bool   // ConnectTimeout remains opt-in
		wantReadValue   string // readTimeout= always present; what value?
		wantWriteValue  string // writeTimeout= always present; what value?
	}{
		{
			name:           "all timeouts set — operator values honored",
			connect:        "30s",
			read:           "10s",
			write:          "15s",
			wantTimeout:    true,
			wantReadValue:  "10s",
			wantWriteValue: "15s",
		},
		{
			name:           "no timeouts — defaults applied to read/write, connect absent",
			connect:        "",
			read:           "",
			write:          "",
			wantTimeout:    false,
			wantReadValue:  "30s",
			wantWriteValue: "30s",
		},
		{
			name:           "only connect timeout — read/write take defaults",
			connect:        "5s",
			read:           "",
			write:          "",
			wantTimeout:    true,
			wantReadValue:  "30s",
			wantWriteValue: "30s",
		},
		{
			name:           "only read timeout — writeTimeout takes default",
			connect:        "",
			read:           "10s",
			write:          "",
			wantTimeout:    false,
			wantReadValue:  "10s",
			wantWriteValue: "30s",
		},
		{
			name:           "only write timeout — readTimeout takes default",
			connect:        "",
			read:           "",
			write:          "15s",
			wantTimeout:    false,
			wantReadValue:  "30s",
			wantWriteValue: "15s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svr := &MySql{
				UserName:       "user",
				Password:       "pass",
				Host:           "localhost",
				Database:       "db",
				ConnectTimeout: tt.connect,
				ReadTimeout:    tt.read,
				WriteTimeout:   tt.write,
			}

			dsn, err := svr.GetDsn()
			if err != nil {
				t.Fatalf("GetDsn() error = %v", err)
			}

			hasTimeout := strings.Contains(dsn, "timeout=")
			if tt.wantTimeout && !hasTimeout {
				t.Errorf("DSN %q missing timeout=", dsn)
			}
			if !tt.wantTimeout && hasTimeout {
				t.Errorf("DSN %q has unexpected timeout=", dsn)
			}

			wantReadKV := "readTimeout=" + tt.wantReadValue
			if !strings.Contains(dsn, wantReadKV) {
				t.Errorf("DSN %q missing %q", dsn, wantReadKV)
			}
			wantWriteKV := "writeTimeout=" + tt.wantWriteValue
			if !strings.Contains(dsn, wantWriteKV) {
				t.Errorf("DSN %q missing %q", dsn, wantWriteKV)
			}
		})
	}
}

func TestGetDsn_RejectReadOnly(t *testing.T) {
	tests := []struct {
		name           string
		rejectReadOnly bool
		wantContains   bool
	}{
		{
			name:           "reject read only true",
			rejectReadOnly: true,
			wantContains:   true,
		},
		{
			name:           "reject read only false",
			rejectReadOnly: false,
			wantContains:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svr := &MySql{
				UserName:       "user",
				Password:       "pass",
				Host:           "localhost",
				Database:       "db",
				RejectReadOnly: tt.rejectReadOnly,
			}

			dsn, err := svr.GetDsn()
			if err != nil {
				t.Fatalf("GetDsn() error = %v", err)
			}

			has := strings.Contains(dsn, "rejectReadOnly=true")
			if tt.wantContains && !has {
				t.Errorf("DSN %q missing rejectReadOnly=true", dsn)
			}
			if !tt.wantContains && has {
				t.Errorf("DSN %q has unexpected rejectReadOnly=true", dsn)
			}
		})
	}
}

func TestGetDsn_SpecialCharactersInCredentials(t *testing.T) {
	// GetDsn does not URL-encode credentials; it uses raw string concatenation.
	// This test documents that behavior.
	svr := &MySql{
		UserName: "user@host",
		Password: "p@ss:word/special",
		Host:     "db.example.com",
		Database: "testdb",
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}

	if !strings.HasPrefix(dsn, "user@host:p@ss:word/special@(") {
		t.Errorf("GetDsn() = %q, expected raw credential concatenation", dsn)
	}
}

func TestGetDsn_WhitespaceOnlyFields(t *testing.T) {
	// Charset and Collation use util.LenTrim (len(strings.TrimSpace(s)))
	// so whitespace-only values should be treated as empty -> use defaults
	svr := &MySql{
		UserName:  "user",
		Password:  "pass",
		Host:      "localhost",
		Database:  "db",
		Charset:   "   ",
		Collation: "   ",
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}

	if !strings.Contains(dsn, "charset=utf8mb4") {
		t.Errorf("whitespace charset should default to utf8mb4, got DSN: %q", dsn)
	}
	if !strings.Contains(dsn, "collation=utf8mb4_general_ci") {
		t.Errorf("whitespace collation should default to utf8mb4_general_ci, got DSN: %q", dsn)
	}
}

func TestGetDsn_ParameterOrdering(t *testing.T) {
	// Verify the DSN query parameters always appear in the correct order
	svr := &MySql{
		UserName:       "user",
		Password:       "pass",
		Host:           "localhost",
		Port:           3306,
		Database:       "db",
		ConnectTimeout: "30s",
		ReadTimeout:    "10s",
		WriteTimeout:   "15s",
		RejectReadOnly: true,
	}

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}

	// Extract query string part
	idx := strings.Index(dsn, "?")
	if idx == -1 {
		t.Fatalf("DSN %q has no query string", dsn)
	}
	query := dsn[idx+1:]

	// Expected order: charset, collation, parseTime, timeout, readTimeout, writeTimeout, rejectReadOnly
	expectedOrder := []string{
		"charset=",
		"collation=",
		"parseTime=",
		"timeout=",
		"readTimeout=",
		"writeTimeout=",
		"rejectReadOnly=",
	}

	lastIdx := -1
	for _, param := range expectedOrder {
		pos := strings.Index(query, param)
		if pos == -1 {
			t.Errorf("DSN query string %q missing %q", query, param)
			continue
		}
		if pos <= lastIdx {
			t.Errorf("parameter %q at position %d should come after position %d in DSN query string", param, pos, lastIdx)
		}
		lastIdx = pos
	}
}

func TestGetDsn_AllEmptyRequired(t *testing.T) {
	// All required fields empty should fail on the first check (UserName)
	svr := &MySql{}
	_, err := svr.GetDsn()
	if err == nil {
		t.Fatal("GetDsn() with all empty fields should return error")
	}
	if !strings.Contains(err.Error(), "User Name is Required") {
		t.Errorf("GetDsn() error = %q, want 'User Name is Required'", err.Error())
	}
}

func TestGetDsn_ValidationOrder(t *testing.T) {
	// Verify that validation checks fields in order: UserName, Password, Host, Database
	// by providing each field one at a time and checking which error comes next
	steps := []struct {
		svr     *MySql
		wantErr string
	}{
		{
			svr:     &MySql{},
			wantErr: "User Name is Required",
		},
		{
			svr:     &MySql{UserName: "u"},
			wantErr: "Password is Required",
		},
		{
			svr:     &MySql{UserName: "u", Password: "p"},
			wantErr: "MySQL Host Address is Required",
		},
		{
			svr:     &MySql{UserName: "u", Password: "p", Host: "h"},
			wantErr: "MySQL Database Name is Required",
		},
	}

	for _, step := range steps {
		_, err := step.svr.GetDsn()
		if err == nil {
			t.Fatalf("expected error %q, got nil", step.wantErr)
		}
		if err.Error() != step.wantErr {
			t.Errorf("error = %q, want %q", err.Error(), step.wantErr)
		}
	}
}

// =====================================================================
// Integration Tests (require live MySQL 8.0 on localhost:3306)
// =====================================================================

// testMySqlConn creates a connected MySql instance for integration tests.
// Skips the test if MySQL is not reachable.
func testMySqlConn(t *testing.T) *MySql {
	t.Helper()
	svr := &MySql{
		UserName: "root",
		Password: "testpass",
		Host:     "localhost",
		Port:     3306,
		Database: "testdb",
	}
	if err := svr.Open(); err != nil {
		t.Skipf("MySQL not available: %v", err)
	}
	t.Cleanup(func() { svr.Close() })
	return svr
}

// testTableName returns a unique table name based on the test name to avoid collisions.
func testTableName(t *testing.T) string {
	t.Helper()
	// Replace slashes and spaces with underscores for subtest-safe table names
	name := strings.ReplaceAll(t.Name(), "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	// MySQL table names limited; keep it reasonable
	if len(name) > 50 {
		name = name[:50]
	}
	return "inttest_" + name
}

// testItem is a simple struct for marshalling DB rows via sqlx db tags.
type testItem struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

func TestMySQL_Integration_OpenPingClose(t *testing.T) {
	svr := testMySqlConn(t)

	// Ping should succeed immediately after Open (within the 90-second cache window
	// Ping has, but Open already called Ping internally, so this verifies the
	// "MySQL Server Not Connected" path is not hit).
	if err := svr.Ping(); err != nil {
		t.Fatalf("Ping() after Open failed: %v", err)
	}

	// Close then verify Ping returns error
	if err := svr.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	err := svr.Ping()
	if err == nil {
		t.Fatal("Ping() after Close should return error, got nil")
	}
	if !strings.Contains(err.Error(), "Not Connected") {
		t.Errorf("Ping() after Close error = %q, want 'Not Connected'", err)
	}
}

func TestMySQL_Integration_ReopenAfterClose(t *testing.T) {
	svr := &MySql{
		UserName: "root",
		Password: "testpass",
		Host:     "localhost",
		Port:     3306,
		Database: "testdb",
	}
	if err := svr.Open(); err != nil {
		t.Skipf("MySQL not available: %v", err)
	}

	// Close
	if err := svr.Close(); err != nil {
		t.Fatalf("first Close() failed: %v", err)
	}

	// Re-open
	if err := svr.Open(); err != nil {
		t.Fatalf("re-Open() after Close failed: %v", err)
	}
	t.Cleanup(func() { svr.Close() })

	if err := svr.Ping(); err != nil {
		t.Fatalf("Ping() after re-Open failed: %v", err)
	}
}

func TestMySQL_Integration_GetDsnFromOpenedConn(t *testing.T) {
	svr := testMySqlConn(t)

	dsn, err := svr.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() error = %v", err)
	}

	// Verify the DSN encodes the connection parameters we set
	wantParts := []string{
		"root:testpass@(localhost:3306)/testdb",
		"charset=utf8mb4",
		"collation=utf8mb4_general_ci",
		"parseTime=true",
	}

	for _, part := range wantParts {
		if !strings.Contains(dsn, part) {
			t.Errorf("GetDsn() = %q, missing expected substring %q", dsn, part)
		}
	}
}

func TestMySQL_Integration_ExecAndGetStruct(t *testing.T) {
	svr := testMySqlConn(t)
	tbl := testTableName(t)
	t.Cleanup(func() {
		svr.ExecByOrdinalParams(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tbl))
	})

	// CREATE TABLE
	createSQL := fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL)", tbl)
	res := svr.ExecByOrdinalParams(createSQL)
	if res.Err != nil {
		t.Fatalf("CREATE TABLE failed: %v", res.Err)
	}

	// INSERT
	insertSQL := fmt.Sprintf("INSERT INTO `%s` (name) VALUES (?)", tbl)
	res = svr.ExecByOrdinalParams(insertSQL, "alice")
	if res.Err != nil {
		t.Fatalf("INSERT failed: %v", res.Err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("INSERT RowsAffected = %d, want 1", res.RowsAffected)
	}
	if res.NewlyInsertedID < 1 {
		t.Errorf("INSERT NewlyInsertedID = %d, want >= 1", res.NewlyInsertedID)
	}

	// GetStruct — retrieve the inserted row
	var item testItem
	selectSQL := fmt.Sprintf("SELECT id, name FROM `%s` WHERE name = ?", tbl)
	notFound, err := svr.GetStruct(&item, selectSQL, "alice")
	if err != nil {
		t.Fatalf("GetStruct() error = %v", err)
	}
	if notFound {
		t.Fatal("GetStruct() returned notFound = true, expected row")
	}
	if item.Name != "alice" {
		t.Errorf("GetStruct() name = %q, want %q", item.Name, "alice")
	}
	if item.ID < 1 {
		t.Errorf("GetStruct() id = %d, want >= 1", item.ID)
	}
}

func TestMySQL_Integration_GetStructSlice(t *testing.T) {
	svr := testMySqlConn(t)
	tbl := testTableName(t)
	t.Cleanup(func() {
		svr.ExecByOrdinalParams(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tbl))
	})

	// Setup
	svr.ExecByOrdinalParams(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL)", tbl))
	svr.ExecByOrdinalParams(fmt.Sprintf("INSERT INTO `%s` (name) VALUES (?), (?), (?)", tbl), "bob", "carol", "dave")

	// GetStructSlice
	var items []testItem
	selectSQL := fmt.Sprintf("SELECT id, name FROM `%s` ORDER BY name ASC", tbl)
	notFound, err := svr.GetStructSlice(&items, selectSQL)
	if err != nil {
		t.Fatalf("GetStructSlice() error = %v", err)
	}
	if notFound {
		t.Fatal("GetStructSlice() returned notFound = true, expected rows")
	}
	if len(items) != 3 {
		t.Fatalf("GetStructSlice() returned %d rows, want 3", len(items))
	}
	wantNames := []string{"bob", "carol", "dave"}
	for i, want := range wantNames {
		if items[i].Name != want {
			t.Errorf("items[%d].Name = %q, want %q", i, items[i].Name, want)
		}
	}
}

func TestMySQL_Integration_GetStructSlice_NotFound(t *testing.T) {
	svr := testMySqlConn(t)
	tbl := testTableName(t)
	t.Cleanup(func() {
		svr.ExecByOrdinalParams(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tbl))
	})

	svr.ExecByOrdinalParams(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL)", tbl))

	// Query empty table — GetStructSlice returns empty slice, not notFound
	var items []testItem
	selectSQL := fmt.Sprintf("SELECT id, name FROM `%s`", tbl)
	notFound, err := svr.GetStructSlice(&items, selectSQL)
	if err != nil {
		t.Fatalf("GetStructSlice() on empty table error = %v", err)
	}
	// sqlx.Select does not return sql.ErrNoRows for empty results; it returns empty slice
	if notFound {
		// This is acceptable behavior — just document it
		t.Logf("GetStructSlice() returned notFound=true for empty table (no rows)")
	}
	if !notFound && len(items) != 0 {
		t.Errorf("GetStructSlice() returned %d items for empty table, expected 0", len(items))
	}
}

func TestMySQL_Integration_GetSingleRow_ScanColumnsByRow(t *testing.T) {
	svr := testMySqlConn(t)
	tbl := testTableName(t)
	t.Cleanup(func() {
		svr.ExecByOrdinalParams(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tbl))
	})

	svr.ExecByOrdinalParams(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL)", tbl))
	svr.ExecByOrdinalParams(fmt.Sprintf("INSERT INTO `%s` (name) VALUES (?)", tbl), "eve")

	// GetSingleRow
	selectSQL := fmt.Sprintf("SELECT id, name FROM `%s` WHERE name = ?", tbl)
	row, err := svr.GetSingleRow(selectSQL, "eve")
	if err != nil {
		t.Fatalf("GetSingleRow() error = %v", err)
	}
	if row == nil {
		t.Fatal("GetSingleRow() returned nil row, expected data")
	}

	// ScanColumnsByRow
	var id int
	var name string
	notFound, err := svr.ScanColumnsByRow(row, &id, &name)
	if err != nil {
		t.Fatalf("ScanColumnsByRow() error = %v", err)
	}
	if notFound {
		t.Fatal("ScanColumnsByRow() notFound = true, expected data")
	}
	if name != "eve" {
		t.Errorf("ScanColumnsByRow() name = %q, want %q", name, "eve")
	}
	if id < 1 {
		t.Errorf("ScanColumnsByRow() id = %d, want >= 1", id)
	}
}

func TestMySQL_Integration_GetScalarString(t *testing.T) {
	svr := testMySqlConn(t)
	tbl := testTableName(t)
	t.Cleanup(func() {
		svr.ExecByOrdinalParams(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tbl))
	})

	svr.ExecByOrdinalParams(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL)", tbl))
	svr.ExecByOrdinalParams(fmt.Sprintf("INSERT INTO `%s` (name) VALUES (?)", tbl), "frank")

	// GetScalarString — returns first column of first row as string
	selectSQL := fmt.Sprintf("SELECT name FROM `%s` WHERE id = ?", tbl)
	val, notFound, err := svr.GetScalarString(selectSQL, 1)
	if err != nil {
		t.Fatalf("GetScalarString() error = %v", err)
	}
	if notFound {
		t.Fatal("GetScalarString() notFound = true, expected value")
	}
	if val != "frank" {
		t.Errorf("GetScalarString() = %q, want %q", val, "frank")
	}

	// GetScalarString — not found case
	val2, notFound2, err2 := svr.GetScalarString(selectSQL, 9999)
	if err2 != nil {
		t.Fatalf("GetScalarString() not-found case error = %v", err2)
	}
	if !notFound2 {
		t.Errorf("GetScalarString() notFound = false for non-existent row, val = %q", val2)
	}
}

func TestMySQL_Integration_GetRowsByOrdinalParams(t *testing.T) {
	svr := testMySqlConn(t)
	tbl := testTableName(t)
	t.Cleanup(func() {
		svr.ExecByOrdinalParams(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tbl))
	})

	svr.ExecByOrdinalParams(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL)", tbl))
	svr.ExecByOrdinalParams(fmt.Sprintf("INSERT INTO `%s` (name) VALUES (?), (?)", tbl), "grace", "heidi")

	selectSQL := fmt.Sprintf("SELECT id, name FROM `%s` ORDER BY name ASC", tbl)
	rows, err := svr.GetRowsByOrdinalParams(selectSQL)
	if err != nil {
		t.Fatalf("GetRowsByOrdinalParams() error = %v", err)
	}
	if rows == nil {
		t.Fatal("GetRowsByOrdinalParams() returned nil rows")
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var id int
		var name string
		if scanErr := rows.Scan(&id, &name); scanErr != nil {
			t.Fatalf("rows.Scan() error = %v", scanErr)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration error = %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("got %d rows, want 2", len(names))
	}
	if names[0] != "grace" || names[1] != "heidi" {
		t.Errorf("rows = %v, want [grace heidi]", names)
	}
}

func TestMySQL_Integration_Transaction_Commit(t *testing.T) {
	svr := testMySqlConn(t)
	tbl := testTableName(t)
	t.Cleanup(func() {
		svr.ExecByOrdinalParams(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tbl))
	})

	svr.ExecByOrdinalParams(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL)", tbl))

	// Begin transaction
	tx, err := svr.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	// INSERT within transaction
	insertSQL := fmt.Sprintf("INSERT INTO `%s` (name) VALUES (?)", tbl)
	txRes := tx.ExecByOrdinalParams(insertSQL, "tx_commit_row")
	if txRes.Err != nil {
		t.Fatalf("tx.ExecByOrdinalParams() error = %v", txRes.Err)
	}

	// Commit
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	// Verify row exists after commit
	val, notFound, err := svr.GetScalarString(fmt.Sprintf("SELECT name FROM `%s` WHERE name = ?", tbl), "tx_commit_row")
	if err != nil {
		t.Fatalf("verification query error = %v", err)
	}
	if notFound {
		t.Fatal("committed row not found after Commit()")
	}
	if val != "tx_commit_row" {
		t.Errorf("committed row name = %q, want %q", val, "tx_commit_row")
	}
}

func TestMySQL_Integration_Transaction_Rollback(t *testing.T) {
	svr := testMySqlConn(t)
	tbl := testTableName(t)
	t.Cleanup(func() {
		svr.ExecByOrdinalParams(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tbl))
	})

	svr.ExecByOrdinalParams(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL)", tbl))

	// Begin transaction
	tx, err := svr.Begin()
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	// INSERT within transaction
	insertSQL := fmt.Sprintf("INSERT INTO `%s` (name) VALUES (?)", tbl)
	txRes := tx.ExecByOrdinalParams(insertSQL, "tx_rollback_row")
	if txRes.Err != nil {
		t.Fatalf("tx.ExecByOrdinalParams() error = %v", txRes.Err)
	}

	// Rollback
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// Verify row does NOT exist after rollback
	_, notFound, err := svr.GetScalarString(fmt.Sprintf("SELECT name FROM `%s` WHERE name = ?", tbl), "tx_rollback_row")
	if err != nil {
		t.Fatalf("verification query error = %v", err)
	}
	if !notFound {
		t.Fatal("rolled-back row should not exist, but was found")
	}
}

func TestMySQL_Integration_ExecOnClosedConnection(t *testing.T) {
	svr := &MySql{
		UserName: "root",
		Password: "testpass",
		Host:     "localhost",
		Port:     3306,
		Database: "testdb",
	}
	if err := svr.Open(); err != nil {
		t.Skipf("MySQL not available: %v", err)
	}

	// Close the connection first
	if err := svr.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// ExecByOrdinalParams should fail on closed connection
	res := svr.ExecByOrdinalParams("SELECT 1")
	if res.Err == nil {
		t.Fatal("ExecByOrdinalParams on closed connection should return error")
	}
	if !strings.Contains(res.Err.Error(), "Not Connected") {
		t.Errorf("error = %q, want containing 'Not Connected'", res.Err)
	}
}

func TestMySQL_Integration_QueryNonExistentTable(t *testing.T) {
	svr := testMySqlConn(t)

	// Query a table that does not exist
	var items []testItem
	_, err := svr.GetStructSlice(&items, "SELECT id, name FROM nonexistent_table_xyz_999")
	if err == nil {
		t.Fatal("query on non-existent table should return error, got nil")
	}
	// MySQL error should mention the table doesn't exist
	if !strings.Contains(strings.ToLower(err.Error()), "doesn't exist") && !strings.Contains(strings.ToLower(err.Error()), "not exist") {
		t.Logf("query error (non-existent table) = %q (accepted as non-nil error)", err)
	}
}

func TestMySQL_Integration_OpenWrongCredentials(t *testing.T) {
	svr := &MySql{
		UserName: "root",
		Password: "WRONG_PASSWORD_XYZ",
		Host:     "localhost",
		Port:     3306,
		Database: "testdb",
	}

	err := svr.Open()
	if err == nil {
		// If MySQL allows empty-auth or something unexpected, clean up
		svr.Close()
		t.Fatal("Open() with wrong password should return error, got nil")
	}
	// Expect an access-denied type error
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "access denied") && !strings.Contains(errLower, "denied") {
		t.Logf("Open() wrong-creds error = %q (accepted as non-nil error)", err)
	}
}

func TestMySQL_Integration_ExecByNamedMapParam(t *testing.T) {
	svr := testMySqlConn(t)
	tbl := testTableName(t)
	t.Cleanup(func() {
		svr.ExecByOrdinalParams(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tbl))
	})

	svr.ExecByOrdinalParams(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL)", tbl))

	// INSERT using named map param
	insertSQL := fmt.Sprintf("INSERT INTO `%s` (name) VALUES (:name)", tbl)
	params := map[string]interface{}{
		"name": "named_param_test",
	}
	res := svr.ExecByNamedMapParam(insertSQL, params)
	if res.Err != nil {
		t.Fatalf("ExecByNamedMapParam() error = %v", res.Err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("ExecByNamedMapParam() RowsAffected = %d, want 1", res.RowsAffected)
	}

	// Verify the row
	val, notFound, err := svr.GetScalarString(fmt.Sprintf("SELECT name FROM `%s` WHERE name = ?", tbl), "named_param_test")
	if err != nil {
		t.Fatalf("verification error = %v", err)
	}
	if notFound {
		t.Fatal("named-param inserted row not found")
	}
	if val != "named_param_test" {
		t.Errorf("verification value = %q, want %q", val, "named_param_test")
	}
}
