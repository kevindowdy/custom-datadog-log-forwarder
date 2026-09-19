// Package forwarder tests for reading the source log file and building log items.
package forwarder

// Import os for filesystem setup in tests.
import "os"

// Import path/filepath to build test file paths.
import "path/filepath"

// Import testing for the standard test framework.
import "testing"

// Import time to construct fixed "now" values for deterministic tests.
import "time"

// Import this module's config package to build a Config for ParseLogs.
import "github.com/kevindowdy/custom-datadog-log-forwarder/src/internal/config"

// TestReadCompleteLinesDropsUnterminatedLastLine checks a torn final write is skipped.
func TestReadCompleteLinesDropsUnterminatedLastLine(t *testing.T) {
	// Build a temp file whose last line has no trailing newline yet.
	path := filepath.Join(t.TempDir(), "log.txt")
	content := "line one\nline two\npartial-third-line-still-being-written"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	// Read the complete lines from the file.
	lines, err := readCompleteLines(path)
	// Fail if reading returned an unexpected error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the two newline-terminated lines should be returned.
	if len(lines) != 2 {
		t.Fatalf("expected 2 complete lines, got %d: %v", len(lines), lines)
	}
	if string(lines[0]) != "line one" || string(lines[1]) != "line two" {
		t.Errorf("unexpected lines: %q, %q", lines[0], lines[1])
	}
}

// TestReadCompleteLinesHandlesTrailingNewline checks a fully-flushed file loses no lines.
func TestReadCompleteLinesHandlesTrailingNewline(t *testing.T) {
	// Build a temp file where every line, including the last, ends with a newline.
	path := filepath.Join(t.TempDir(), "log.txt")
	if err := os.WriteFile(path, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	// Read the complete lines from the file.
	lines, err := readCompleteLines(path)
	// Fail if reading returned an unexpected error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both fully-terminated lines should be returned.
	if len(lines) != 2 {
		t.Fatalf("expected 2 complete lines, got %d: %v", len(lines), lines)
	}
}

// TestParseLogsMissingFileReturnsEmptyResult checks a not-yet-created daily file isn't an error.
func TestParseLogsMissingFileReturnsEmptyResult(t *testing.T) {
	// Point the pattern at a file that will never exist in this temp directory.
	dir := t.TempDir()
	cfg := config.Config{LogDir: dir, LogFileNamePattern: "app_20060102.log"}
	// Parsing should succeed with an empty result, not an error.
	result, err := ParseLogs(cfg, map[string]struct{}{}, time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Logs) != 0 || len(result.Warnings) != 0 {
		t.Errorf("expected an empty result, got %+v", result)
	}
}

// TestParseLogsDirectoryDigitsAreNotReformatted is a regression test: the log
// directory must never be run through time.Format, since an unrelated digit
// sequence in a directory name (timestamps, ports, IDs) could otherwise be
// silently misinterpreted as a reference-time token (e.g. "15" as the hour).
func TestParseLogsDirectoryDigitsAreNotReformatted(t *testing.T) {
	// Build a directory name containing "15", which collides with the Go
	// reference-time hour token if it were ever passed through time.Format.
	parent := t.TempDir()
	dir := filepath.Join(parent, "run-1558969979")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	// Fix "now" at midnight so a corrupted hour token would be easy to spot.
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	// Write today's log file under the digit-bearing directory, containing one new entry.
	logPath := filepath.Join(dir, "app_"+now.Format("20060102")+".log")
	line := `{"timestamp":"t","level":"INFO","log_id":"id-1","message":"hi","application":"app"}` + "\n"
	if err := os.WriteFile(logPath, []byte(line), 0o644); err != nil {
		t.Fatalf("failed to seed log file: %v", err)
	}
	// Parse using the directory (with digits) and a filename-only pattern.
	cfg := config.Config{LogDir: dir, LogFileNamePattern: "app_20060102.log"}
	result, err := ParseLogs(cfg, map[string]struct{}{}, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The entry must be found; if the directory had been reformatted, the
	// resolved path would point at a nonexistent file and this would be empty.
	if len(result.Logs) != 1 || result.Logs[0].LogID != "id-1" {
		t.Fatalf("expected to find the seeded log via the digit-bearing directory, got %+v", result)
	}
}

// TestParseLogsSkipsAlreadySentAndMalformedLines checks the core dedup + robustness behavior.
func TestParseLogsSkipsAlreadySentAndMalformedLines(t *testing.T) {
	// Build today's log file path exactly as ParseLogs will resolve it.
	dir := t.TempDir()
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	logPath := filepath.Join(dir, "app_"+now.Format("20060102")+".log")

	// Write a mix of: an already-sent entry, a new entry, a malformed line, a
	// blank line, and a final line with no trailing newline (still being written).
	content := "" +
		`{"timestamp":"t","level":"INFO","log_id":"already-sent","message":"old","application":"app"}` + "\n" +
		`{"timestamp":"t","level":"INFO","log_id":"new-1","message":"new","application":"app"}` + "\n" +
		"not valid json\n" +
		"\n" +
		`{"timestamp":"t","level":"ERROR","log_id":"partial`
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to seed log file: %v", err)
	}

	// Build a Config whose pattern resolves to logPath for the fixed `now`.
	cfg := config.Config{LogDir: dir, LogFileNamePattern: "app_20060102.log"}
	// Mark "already-sent" as already forwarded in an earlier run.
	alreadySent := map[string]struct{}{"already-sent": {}}

	// Parse the log file.
	result, err := ParseLogs(cfg, alreadySent, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the one new, well-formed, not-yet-sent entry should come back.
	if len(result.Logs) != 1 {
		t.Fatalf("expected 1 new log, got %d: %+v", len(result.Logs), result.Logs)
	}
	if result.Logs[0].LogID != "new-1" {
		t.Errorf("LogID = %q, want %q", result.Logs[0].LogID, "new-1")
	}
	// The malformed line should have produced exactly one warning.
	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning for the malformed line, got %d: %v", len(result.Warnings), result.Warnings)
	}
}
