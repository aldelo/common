package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitWithAppNameConsole verifies that Init succeeds when AppName is set
// and OutputToConsole is true (no file I/O needed).
func TestInitWithAppNameConsole(t *testing.T) {
	z := &ZapLog{
		AppName:         "test-app",
		OutputToConsole: true,
	}

	err := z.Init()
	if err != nil {
		t.Fatalf("Init() returned unexpected error: %v", err)
	}

	// Clean up
	_ = z.Sync()
}

// TestInitWithEmptyAppName verifies that Init returns an error when
// AppName is empty.
func TestInitWithEmptyAppName(t *testing.T) {
	z := &ZapLog{
		OutputToConsole: true,
	}

	err := z.Init()
	if err == nil {
		t.Fatal("Init() should return error when AppName is empty")
	}

	if !strings.Contains(err.Error(), "App Name is Required") {
		t.Errorf("Error message = %q, expected to contain 'App Name is Required'", err.Error())
	}
}

// TestInitWithWhitespaceOnlyAppName verifies that Init returns an error when
// AppName contains only whitespace (LenTrim should return 0).
func TestInitWithWhitespaceOnlyAppName(t *testing.T) {
	z := &ZapLog{
		AppName:         "   ",
		OutputToConsole: true,
	}

	err := z.Init()
	if err == nil {
		t.Fatal("Init() should return error when AppName is only whitespace")
	}
}

// TestInitWithOutputToFile verifies that Init with OutputToFile creates a log file
// in the current directory. We use a temp directory to avoid polluting the source tree.
func TestInitWithOutputToFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp directory so log files are created there
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to chdir to temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	z := &ZapLog{
		AppName:         "testfileapp",
		OutputToConsole: false,
	}

	if err := z.Init(); err != nil {
		t.Fatalf("Init() returned unexpected error: %v", err)
	}

	// Write a log entry to force file creation
	z.Infof("test log message %s", "hello")
	_ = z.Sync()

	// Verify the log file was created
	logFile := filepath.Join(tmpDir, "testfileapp.log")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("Expected log file %s to be created", logFile)
	}
}

// TestLogMethodsDoNotPanicAfterInit verifies that calling log methods
// after Init does not panic.
func TestLogMethodsDoNotPanicAfterInit(t *testing.T) {
	z := &ZapLog{
		AppName:         "test-nopanic",
		OutputToConsole: true,
	}

	if err := z.Init(); err != nil {
		t.Fatalf("Init() returned unexpected error: %v", err)
	}

	// These should not panic
	z.Infof("info message %s", "test")
	z.Infow("info kv", "key", "value")
	z.Debugf("debug message %s", "test")
	z.Debugw("debug kv", "key", "value")
	z.Warnf("warn message %s", "test")
	z.Warnw("warn kv", "key", "value")
	z.Errorf("error message %s", "test")
	z.Errorw("error kv", "key", "value")

	_ = z.Sync()
}

// TestLogMethodsWithDisabledLogger verifies that calling log methods
// with DisableLogger=true does not panic and effectively no-ops.
func TestLogMethodsWithDisabledLogger(t *testing.T) {
	z := &ZapLog{
		AppName:         "test-disabled",
		OutputToConsole: true,
		DisableLogger:   true,
	}

	if err := z.Init(); err != nil {
		t.Fatalf("Init() returned unexpected error: %v", err)
	}

	// These should be no-ops, not panics
	z.Infof("should not log %s", "test")
	z.Infow("should not log", "key", "value")
	z.Debugf("should not log %s", "test")
	z.Debugw("should not log", "key", "value")
	z.Warnf("should not log %s", "test")
	z.Warnw("should not log", "key", "value")
	z.Errorf("should not log %s", "test")
	z.Errorw("should not log", "key", "value")

	_ = z.Sync()
}

// TestLogMethodsBeforeInit verifies that calling log methods before Init
// does not panic (nil logger guards in each method).
func TestLogMethodsBeforeInit(t *testing.T) {
	z := &ZapLog{
		AppName:         "test-noinit",
		OutputToConsole: true,
	}

	// These should not panic even without Init
	z.Infof("before init %s", "test")
	z.Infow("before init", "key", "value")
	z.Debugf("before init %s", "test")
	z.Debugw("before init", "key", "value")
	z.Warnf("before init %s", "test")
	z.Warnw("before init", "key", "value")
	z.Errorf("before init %s", "test")
	z.Errorw("before init", "key", "value")
}

// TestNilZapLogInit verifies that Init on a nil ZapLog returns an error.
func TestNilZapLogInit(t *testing.T) {
	var z *ZapLog
	err := z.Init()
	if err == nil {
		t.Fatal("Init() on nil ZapLog should return error")
	}
	if !strings.Contains(err.Error(), "ZapLog is nil") {
		t.Errorf("Error message = %q, expected to contain 'ZapLog is nil'", err.Error())
	}
}

// TestSyncWithoutInit verifies that Sync on an un-initialized ZapLog
// does not panic and returns nil.
func TestSyncWithoutInit(t *testing.T) {
	z := &ZapLog{}
	err := z.Sync()
	if err != nil {
		t.Errorf("Sync() on un-initialized ZapLog returned error: %v", err)
	}
}

// TestReInit verifies that calling Init() a second time does not panic
// and properly replaces the logger.
func TestReInit(t *testing.T) {
	z := &ZapLog{
		AppName:         "test-reinit",
		OutputToConsole: true,
	}

	if err := z.Init(); err != nil {
		t.Fatalf("First Init() failed: %v", err)
	}

	z.Infof("first logger message")

	// Re-initialize
	if err := z.Init(); err != nil {
		t.Fatalf("Second Init() failed: %v", err)
	}

	z.Infof("second logger message")
	_ = z.Sync()
}

// TestPrintfIsAliasForInfof verifies that Printf does not panic
// (it is documented as an alias for Infof).
func TestPrintfIsAliasForInfof(t *testing.T) {
	z := &ZapLog{
		AppName:         "test-printf",
		OutputToConsole: true,
	}

	if err := z.Init(); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Should not panic
	z.Printf("printf message %s", "test")
	_ = z.Sync()
}
