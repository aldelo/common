package sqlite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tempDBPath creates a temporary SQLite database file and returns its path.
// The caller is responsible for removing the file when done.
func tempDBPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

// openTestDB creates and opens a SQLite database for testing purposes.
func openTestDB(t *testing.T) (*SQLite, string) {
	t.Helper()
	dbPath := tempDBPath(t)
	s := &SQLite{
		DatabasePath:     dbPath,
		PingFrequencySec: -1, // always ping for test reliability
	}
	if err := s.Open(); err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return s, dbPath
}

// ----------------------------------------------------------------------------------------------------------------
// A1: GetDsn locking mode tests
// ----------------------------------------------------------------------------------------------------------------

func TestGetDsn_DefaultLockingMode(t *testing.T) {
	s := &SQLite{
		DatabasePath: "/tmp/test.db",
	}
	dsn, err := s.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() returned error: %v", err)
	}
	if !strings.Contains(dsn, "_locking_mode=EXCLUSIVE") {
		t.Errorf("expected DSN to contain _locking_mode=EXCLUSIVE, got: %s", dsn)
	}
}

func TestGetDsn_CustomLockingMode(t *testing.T) {
	s := &SQLite{
		DatabasePath: "/tmp/test.db",
		LockingMode:  "NORMAL",
	}
	dsn, err := s.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() returned error: %v", err)
	}
	if !strings.Contains(dsn, "_locking_mode=NORMAL") {
		t.Errorf("expected DSN to contain _locking_mode=NORMAL, got: %s", dsn)
	}
	if strings.Contains(dsn, "_locking_mode=EXCLUSIVE") {
		t.Errorf("DSN should not contain EXCLUSIVE when NORMAL is set, got: %s", dsn)
	}
}

func TestGetDsn_WhitespaceLockingMode(t *testing.T) {
	// A whitespace-only LockingMode should be treated as empty (default to EXCLUSIVE)
	s := &SQLite{
		DatabasePath: "/tmp/test.db",
		LockingMode:  "   ",
	}
	dsn, err := s.GetDsn()
	if err != nil {
		t.Fatalf("GetDsn() returned error: %v", err)
	}
	if !strings.Contains(dsn, "_locking_mode=EXCLUSIVE") {
		t.Errorf("expected whitespace LockingMode to default to EXCLUSIVE, got: %s", dsn)
	}
}

// ----------------------------------------------------------------------------------------------------------------
// A3: Ping caching tests
// ----------------------------------------------------------------------------------------------------------------

func TestPing_CachingSkipsSecondPing(t *testing.T) {
	s, dbPath := openTestDB(t)
	defer s.Close()
	defer os.Remove(dbPath)

	// Set a large ping frequency so second ping is cached
	s.PingFrequencySec = 60

	// Force an initial ping to set lastPing
	s.mu.Lock()
	s.lastPing = time.Now()
	s.mu.Unlock()

	// The second ping should be nearly instant because it is cached
	start := time.Now()
	err := s.Ping()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("cached Ping() returned error: %v", err)
	}

	// A cached ping should complete very quickly (no actual DB round-trip)
	// We allow a generous 50ms threshold to avoid flaky tests
	if elapsed > 50*time.Millisecond {
		t.Errorf("cached Ping() took %v, expected near-instant", elapsed)
	}
}

func TestPing_NegativeFrequencyAlwaysPings(t *testing.T) {
	s, dbPath := openTestDB(t)
	defer s.Close()
	defer os.Remove(dbPath)

	// PingFrequencySec < 0 means always ping
	s.PingFrequencySec = -1

	// Set lastPing to now (so cache window would normally skip)
	s.mu.Lock()
	s.lastPing = time.Now()
	s.mu.Unlock()

	// This should still perform an actual ping
	err := s.Ping()
	if err != nil {
		t.Fatalf("Ping() with negative frequency returned error: %v", err)
	}
}

// ----------------------------------------------------------------------------------------------------------------
// A5: BeginTx + Commit round-trip
// ----------------------------------------------------------------------------------------------------------------

func TestBeginTx_CommitRoundTrip(t *testing.T) {
	s, dbPath := openTestDB(t)
	defer s.Close()
	defer os.Remove(dbPath)

	// Create a test table
	res := s.ExecByOrdinalParams("CREATE TABLE test_tx (id INTEGER PRIMARY KEY, name TEXT)")
	if res.Err != nil {
		t.Fatalf("CREATE TABLE failed: %v", res.Err)
	}

	// Start a named transaction
	tx, err := s.BeginTx("commit-test")
	if err != nil {
		t.Fatalf("BeginTx() failed: %v", err)
	}

	if tx.ID() != "commit-test" {
		t.Errorf("expected tx ID 'commit-test', got '%s'", tx.ID())
	}

	// Insert within the transaction
	insertRes := tx.ExecByOrdinalParams("INSERT INTO test_tx (name) VALUES (?)", "alice")
	if insertRes.Err != nil {
		t.Fatalf("INSERT in tx failed: %v", insertRes.Err)
	}
	if insertRes.NewlyInsertedID == 0 {
		t.Error("expected non-zero NewlyInsertedID after INSERT")
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	// Verify the row was committed
	val, notFound, err := s.GetScalarString("SELECT name FROM test_tx WHERE id=1")
	if err != nil {
		t.Fatalf("GetScalarString() failed: %v", err)
	}
	if notFound {
		t.Error("expected to find committed row, got notFound=true")
	}
	if val != "alice" {
		t.Errorf("expected 'alice', got '%s'", val)
	}

	// Verify txMap is cleaned up
	s.mu.RLock()
	_, exists := s.txMap["commit-test"]
	s.mu.RUnlock()
	if exists {
		t.Error("expected transaction to be removed from txMap after commit")
	}
}

// ----------------------------------------------------------------------------------------------------------------
// A5: BeginTx + Rollback round-trip
// ----------------------------------------------------------------------------------------------------------------

func TestBeginTx_RollbackRoundTrip(t *testing.T) {
	s, dbPath := openTestDB(t)
	defer s.Close()
	defer os.Remove(dbPath)

	// Create a test table
	res := s.ExecByOrdinalParams("CREATE TABLE test_rollback (id INTEGER PRIMARY KEY, name TEXT)")
	if res.Err != nil {
		t.Fatalf("CREATE TABLE failed: %v", res.Err)
	}

	// Insert a baseline row outside transaction
	res = s.ExecByOrdinalParams("INSERT INTO test_rollback (name) VALUES (?)", "baseline")
	if res.Err != nil {
		t.Fatalf("baseline INSERT failed: %v", res.Err)
	}

	// Start a named transaction
	tx, err := s.BeginTx("rollback-test")
	if err != nil {
		t.Fatalf("BeginTx() failed: %v", err)
	}

	// Insert within the transaction
	insertRes := tx.ExecByOrdinalParams("INSERT INTO test_rollback (name) VALUES (?)", "should-vanish")
	if insertRes.Err != nil {
		t.Fatalf("INSERT in tx failed: %v", insertRes.Err)
	}

	// Rollback the transaction
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() failed: %v", err)
	}

	// Verify the rolled-back row does not exist
	_, notFound, err := s.GetScalarString("SELECT name FROM test_rollback WHERE name='should-vanish'")
	if err != nil {
		t.Fatalf("GetScalarString() failed: %v", err)
	}
	if !notFound {
		t.Error("expected rolled-back row to not be found")
	}

	// Verify the baseline row still exists
	val, notFound, err := s.GetScalarString("SELECT name FROM test_rollback WHERE name='baseline'")
	if err != nil {
		t.Fatalf("GetScalarString() for baseline failed: %v", err)
	}
	if notFound {
		t.Error("expected baseline row to exist")
	}
	if val != "baseline" {
		t.Errorf("expected 'baseline', got '%s'", val)
	}

	// Verify txMap is cleaned up
	s.mu.RLock()
	_, exists := s.txMap["rollback-test"]
	s.mu.RUnlock()
	if exists {
		t.Error("expected transaction to be removed from txMap after rollback")
	}
}

// ----------------------------------------------------------------------------------------------------------------
// A5: Close() rollbacks outstanding transactions
// ----------------------------------------------------------------------------------------------------------------

func TestClose_RollbacksOutstandingTransactions(t *testing.T) {
	s, dbPath := openTestDB(t)
	defer os.Remove(dbPath)

	// Create a test table
	res := s.ExecByOrdinalParams("CREATE TABLE test_close (id INTEGER PRIMARY KEY, name TEXT)")
	if res.Err != nil {
		t.Fatalf("CREATE TABLE failed: %v", res.Err)
	}

	// Start a named transaction but do NOT commit
	tx, err := s.BeginTx("outstanding")
	if err != nil {
		t.Fatalf("BeginTx() failed: %v", err)
	}

	// Insert within the open transaction
	insertRes := tx.ExecByOrdinalParams("INSERT INTO test_close (name) VALUES (?)", "uncommitted")
	if insertRes.Err != nil {
		t.Fatalf("INSERT in tx failed: %v", insertRes.Err)
	}

	// Close the database -- this should rollback outstanding transactions
	if err := s.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Verify the transaction was marked as closed
	tx.mu.Lock()
	closed := tx.closed
	tx.mu.Unlock()
	if !closed {
		t.Error("expected outstanding transaction to be closed after Close()")
	}

	// Reopen and verify the uncommitted data is gone
	s2 := &SQLite{
		DatabasePath:     dbPath,
		PingFrequencySec: -1,
	}
	if err := s2.Open(); err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer s2.Close()

	_, notFound, err := s2.GetScalarString("SELECT name FROM test_close WHERE name='uncommitted'")
	if err != nil {
		t.Fatalf("GetScalarString() after reopen failed: %v", err)
	}
	if !notFound {
		t.Error("expected uncommitted row to be gone after Close() rollback")
	}
}

// ----------------------------------------------------------------------------------------------------------------
// A5: BeginTx with empty tag generates an auto-ID
// ----------------------------------------------------------------------------------------------------------------

func TestBeginTx_EmptyTagGeneratesID(t *testing.T) {
	s, dbPath := openTestDB(t)
	defer s.Close()
	defer os.Remove(dbPath)

	tx, err := s.BeginTx("")
	if err != nil {
		t.Fatalf("BeginTx('') failed: %v", err)
	}
	defer tx.Rollback()

	if tx.ID() == "" {
		t.Error("expected auto-generated ID, got empty string")
	}
	if !strings.HasPrefix(tx.ID(), "tx_") {
		t.Errorf("expected auto-generated ID to start with 'tx_', got '%s'", tx.ID())
	}
}

// ----------------------------------------------------------------------------------------------------------------
// A2: Connection pool configuration test
// ----------------------------------------------------------------------------------------------------------------

func TestOpen_SetsConnectionPoolDefaults(t *testing.T) {
	dbPath := tempDBPath(t)
	defer os.Remove(dbPath)

	s := &SQLite{
		DatabasePath:     dbPath,
		MaxOpenConns:     5,
		MaxIdleConns:     2,
		MaxConnIdleTime:  10 * time.Second,
		PingFrequencySec: -1,
	}
	if err := s.Open(); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer s.Close()

	// Verify the db object is not nil (pool settings were applied during Open)
	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()
	if db == nil {
		t.Fatal("expected db to be non-nil after Open()")
	}

	// We cannot directly query the pool settings via database/sql API,
	// but we verify that Open() succeeded without error which means
	// SetMaxOpenConns, SetMaxIdleConns, SetConnMaxIdleTime were called.
	stats := db.Stats()
	if stats.MaxOpenConnections != 5 {
		t.Errorf("expected MaxOpenConnections=5, got %d", stats.MaxOpenConnections)
	}
}
