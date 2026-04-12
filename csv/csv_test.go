package csv

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestCSV is a helper that writes CSV content to a temp file and returns the path.
func writeTestCSV(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "test.csv")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test CSV file: %v", err)
	}
	return path
}

// TestOpenAndReadSimpleCSV verifies opening a CSV file, skipping the header,
// and reading data rows.
func TestOpenAndReadSimpleCSV(t *testing.T) {
	tmpDir := t.TempDir()
	content := "Name,Age,City\nAlice,30,NYC\nBob,25,LA\n"
	path := writeTestCSV(t, tmpDir, content)

	c := &Csv{}
	if err := c.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer c.Close()

	if err := c.SkipHeaderRow(); err != nil {
		t.Fatalf("SkipHeaderRow() failed: %v", err)
	}

	if err := c.BeginCsvReader(); err != nil {
		t.Fatalf("BeginCsvReader() failed: %v", err)
	}

	// Read first row
	eof, record, err := c.ReadCsv()
	if err != nil {
		t.Fatalf("ReadCsv() row 1 failed: %v", err)
	}
	if eof {
		t.Fatal("ReadCsv() row 1 returned eof unexpectedly")
	}
	if len(record) != 3 {
		t.Fatalf("Row 1 has %d fields, expected 3", len(record))
	}
	if record[0] != "Alice" || record[1] != "30" || record[2] != "NYC" {
		t.Errorf("Row 1 = %v, expected [Alice 30 NYC]", record)
	}

	// Read second row
	eof, record, err = c.ReadCsv()
	if err != nil {
		t.Fatalf("ReadCsv() row 2 failed: %v", err)
	}
	if eof {
		t.Fatal("ReadCsv() row 2 returned eof unexpectedly")
	}
	if record[0] != "Bob" || record[1] != "25" || record[2] != "LA" {
		t.Errorf("Row 2 = %v, expected [Bob 25 LA]", record)
	}

	// Read past end - should be EOF
	eof, _, err = c.ReadCsv()
	if err != nil {
		t.Fatalf("ReadCsv() past end failed: %v", err)
	}
	if !eof {
		t.Error("ReadCsv() past end should return eof=true")
	}

	// Verify counters
	if c.ParsedCount != 2 {
		t.Errorf("ParsedCount = %d, expected 2", c.ParsedCount)
	}
	if c.TriedCount != 2 {
		t.Errorf("TriedCount = %d, expected 2", c.TriedCount)
	}
}

// TestEmptyFile verifies behavior when opening an empty CSV file.
func TestEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := writeTestCSV(t, tmpDir, "")

	c := &Csv{}
	if err := c.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer c.Close()

	// SkipHeaderRow on empty file should return error (EOF)
	err := c.SkipHeaderRow()
	if err == nil {
		t.Error("SkipHeaderRow() on empty file should return error")
	}
}

// TestCSVWithQuotedFields verifies parsing of quoted fields that contain
// commas and other special characters.
func TestCSVWithQuotedFields(t *testing.T) {
	tmpDir := t.TempDir()
	// CSV with quoted fields containing commas
	content := "Name,Description\n\"Smith, John\",\"Has a comma, here\"\n"
	path := writeTestCSV(t, tmpDir, content)

	c := &Csv{}
	if err := c.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer c.Close()

	if err := c.SkipHeaderRow(); err != nil {
		t.Fatalf("SkipHeaderRow() failed: %v", err)
	}

	if err := c.BeginCsvReader(); err != nil {
		t.Fatalf("BeginCsvReader() failed: %v", err)
	}

	eof, record, err := c.ReadCsv()
	if err != nil {
		t.Fatalf("ReadCsv() failed: %v", err)
	}
	if eof {
		t.Fatal("ReadCsv() returned eof unexpectedly")
	}
	if len(record) != 2 {
		t.Fatalf("Record has %d fields, expected 2", len(record))
	}
	if record[0] != "Smith, John" {
		t.Errorf("Field 0 = %q, expected %q", record[0], "Smith, John")
	}
	if record[1] != "Has a comma, here" {
		t.Errorf("Field 1 = %q, expected %q", record[1], "Has a comma, here")
	}
}

// TestOpenNonExistentFile verifies that Open returns an error for a
// non-existent file path.
func TestOpenNonExistentFile(t *testing.T) {
	c := &Csv{}
	err := c.Open("/nonexistent/path/to/file.csv")
	if err == nil {
		t.Error("Open() should return error for non-existent file")
	}
}

// TestCloseIdempotent verifies that calling Close multiple times does not panic
// and is safe.
func TestCloseIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	content := "A,B\n1,2\n"
	path := writeTestCSV(t, tmpDir, content)

	c := &Csv{}
	if err := c.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}

	// First close
	if err := c.Close(); err != nil {
		t.Errorf("First Close() returned error: %v", err)
	}

	// Second close should be safe
	if err := c.Close(); err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}
}

// TestCloseOnNilCsv verifies that Close on nil Csv does not panic.
func TestCloseOnNilCsv(t *testing.T) {
	var c *Csv
	err := c.Close()
	if err != nil {
		t.Errorf("Close() on nil Csv returned error: %v", err)
	}
}

// TestReadCsvBeforeBeginCsvReader verifies that ReadCsv returns an error
// when BeginCsvReader has not been called.
func TestReadCsvBeforeBeginCsvReader(t *testing.T) {
	tmpDir := t.TempDir()
	content := "A,B\n1,2\n"
	path := writeTestCSV(t, tmpDir, content)

	c := &Csv{}
	if err := c.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer c.Close()

	if err := c.SkipHeaderRow(); err != nil {
		t.Fatalf("SkipHeaderRow() failed: %v", err)
	}

	// Don't call BeginCsvReader — ReadCsv should error
	_, _, err := c.ReadCsv()
	if err == nil {
		t.Error("ReadCsv() should return error when BeginCsvReader was not called")
	}
}

// TestCSVHeaderOnly verifies behavior with a CSV that only has a header row
// and no data rows.
func TestCSVHeaderOnly(t *testing.T) {
	tmpDir := t.TempDir()
	content := "Col1,Col2,Col3\n"
	path := writeTestCSV(t, tmpDir, content)

	c := &Csv{}
	if err := c.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer c.Close()

	if err := c.SkipHeaderRow(); err != nil {
		t.Fatalf("SkipHeaderRow() failed: %v", err)
	}

	if err := c.BeginCsvReader(); err != nil {
		t.Fatalf("BeginCsvReader() failed: %v", err)
	}

	// Should immediately return EOF
	eof, _, err := c.ReadCsv()
	if err != nil {
		t.Fatalf("ReadCsv() returned error: %v", err)
	}
	if !eof {
		t.Error("ReadCsv() should return eof=true for header-only CSV")
	}
}

// TestMultipleDataRows verifies reading multiple data rows and counter accuracy.
func TestMultipleDataRows(t *testing.T) {
	tmpDir := t.TempDir()
	content := "ID,Val\n1,a\n2,b\n3,c\n4,d\n5,e\n"
	path := writeTestCSV(t, tmpDir, content)

	c := &Csv{}
	if err := c.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer c.Close()

	if err := c.SkipHeaderRow(); err != nil {
		t.Fatalf("SkipHeaderRow() failed: %v", err)
	}

	if err := c.BeginCsvReader(); err != nil {
		t.Fatalf("BeginCsvReader() failed: %v", err)
	}

	rowCount := 0
	for {
		eof, record, err := c.ReadCsv()
		if err != nil {
			t.Fatalf("ReadCsv() failed at row %d: %v", rowCount+1, err)
		}
		if eof {
			break
		}
		rowCount++
		if len(record) != 2 {
			t.Errorf("Row %d has %d fields, expected 2", rowCount, len(record))
		}
	}

	if rowCount != 5 {
		t.Errorf("Read %d rows, expected 5", rowCount)
	}

	if c.ParsedCount != 5 {
		t.Errorf("ParsedCount = %d, expected 5", c.ParsedCount)
	}
	if c.TriedCount != 5 {
		t.Errorf("TriedCount = %d, expected 5", c.TriedCount)
	}
}

// TestReadCsvAfterClose verifies that ReadCsv returns an error after Close.
func TestReadCsvAfterClose(t *testing.T) {
	tmpDir := t.TempDir()
	content := "A\n1\n"
	path := writeTestCSV(t, tmpDir, content)

	c := &Csv{}
	if err := c.Open(path); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}

	if err := c.SkipHeaderRow(); err != nil {
		t.Fatalf("SkipHeaderRow() failed: %v", err)
	}

	if err := c.BeginCsvReader(); err != nil {
		t.Fatalf("BeginCsvReader() failed: %v", err)
	}

	_ = c.Close()

	_, _, err := c.ReadCsv()
	if err == nil {
		t.Error("ReadCsv() after Close() should return error")
	}
}
