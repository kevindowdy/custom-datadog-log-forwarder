// Package forwarder tests for the end-to-end Run orchestration.
package forwarder

// Import errors to build a sentinel failure for a fake sender.
import "errors"

// Import os for filesystem setup in tests.
import "os"

// Import path/filepath to build test file paths.
import "path/filepath"

// Import testing for the standard test framework.
import "testing"

// Import time to construct fixed "now" values for deterministic tests.
import "time"

// Import the Datadog v2 API types used by the fake sender.
import "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

// Import this module's config package to build a Config for Run.
import "github.com/kevindowdy/custom-datadog-log-forwarder/src/internal/config"

// writeLogFile seeds a source log file with the given raw content.
func writeLogFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to seed log file: %v", err)
	}
}

// TestRunSendsNewLogsAndRecordsThemAsSent covers the golden path end to end.
func TestRunSendsNewLogsAndRecordsThemAsSent(t *testing.T) {
	// Build isolated directories for the source log and the dedup tracking store.
	logDir := t.TempDir()
	sentDir := t.TempDir()
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)

	// Seed today's source log file with two new, well-formed entries.
	writeLogFile(t, logDir, "app_20260919.log",
		`{"timestamp":"t","level":"INFO","log_id":"id-1","message":"one","application":"app"}`+"\n"+
			`{"timestamp":"t","level":"INFO","log_id":"id-2","message":"two","application":"app"}`+"\n")

	cfg := config.Config{
		LogDir:             logDir,
		LogFileNamePattern: "app_20060102.log",
		LogsSentDir:        sentDir,
		LogsSentRetention:  7 * 24 * time.Hour,
		BatchMaxItems:      10,
		BatchMaxBytes:      1 << 20,
	}

	// Record what the fake sender receives.
	var sentBatches [][]datadogV2.HTTPLogItem
	sender := func(items []datadogV2.HTTPLogItem) error {
		sentBatches = append(sentBatches, items)
		return nil
	}

	// Run the forwarder once.
	summary, err := Run(cfg, sender, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.NewLogsFound != 2 || summary.LogsSent != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(sentBatches) != 1 || len(sentBatches[0]) != 2 {
		t.Fatalf("expected one batch of 2 items, got %+v", sentBatches)
	}

	// Reload the dedup store and confirm both IDs were persisted as sent.
	store := NewSentStore(sentDir, now)
	sent, err := store.Load()
	if err != nil {
		t.Fatalf("unexpected error loading store: %v", err)
	}
	if _, ok := sent["id-1"]; !ok {
		t.Error("expected id-1 to be recorded as sent")
	}
	if _, ok := sent["id-2"]; !ok {
		t.Error("expected id-2 to be recorded as sent")
	}
}

// TestRunSecondPassSkipsAlreadySentLogs proves the dedup loop actually closes:
// running twice against the same (still-growing) log file never resends a line.
func TestRunSecondPassSkipsAlreadySentLogs(t *testing.T) {
	logDir := t.TempDir()
	sentDir := t.TempDir()
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)

	writeLogFile(t, logDir, "app_20260919.log",
		`{"timestamp":"t","level":"INFO","log_id":"id-1","message":"one","application":"app"}`+"\n")

	cfg := config.Config{
		LogDir: logDir, LogFileNamePattern: "app_20060102.log",
		LogsSentDir: sentDir, LogsSentRetention: 7 * 24 * time.Hour,
		BatchMaxItems: 10, BatchMaxBytes: 1 << 20,
	}

	callCount := 0
	sender := func(items []datadogV2.HTTPLogItem) error {
		callCount++
		return nil
	}

	// First run forwards the one existing line.
	if _, err := Run(cfg, sender, now); err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}
	// Simulate the source app appending one more line before the next scheduled run.
	f, err := os.OpenFile(filepath.Join(logDir, "app_20260919.log"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to reopen log file: %v", err)
	}
	if _, err := f.WriteString(`{"timestamp":"t","level":"INFO","log_id":"id-2","message":"two","application":"app"}` + "\n"); err != nil {
		t.Fatalf("failed to append: %v", err)
	}
	f.Close()

	// Second run must only forward the newly appended line.
	summary, err := Run(cfg, sender, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}
	if summary.NewLogsFound != 1 || summary.LogsSent != 1 {
		t.Fatalf("expected only the new line to be forwarded, got %+v", summary)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 total sender calls (one per run), got %d", callCount)
	}
}

// TestRunPersistsPartialProgressOnSendFailure proves a mid-batch send failure
// still records whatever was confirmed sent, so a retry never double-sends.
func TestRunPersistsPartialProgressOnSendFailure(t *testing.T) {
	logDir := t.TempDir()
	sentDir := t.TempDir()
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)

	writeLogFile(t, logDir, "app_20260919.log",
		`{"timestamp":"t","level":"INFO","log_id":"id-1","message":"one","application":"app"}`+"\n"+
			`{"timestamp":"t","level":"INFO","log_id":"id-2","message":"two","application":"app"}`+"\n")

	cfg := config.Config{
		LogDir: logDir, LogFileNamePattern: "app_20060102.log",
		LogsSentDir: sentDir, LogsSentRetention: 7 * 24 * time.Hour,
		// Force one item per batch so the second batch's failure is isolated.
		BatchMaxItems: 1, BatchMaxBytes: 1 << 20,
	}

	callCount := 0
	sender := func(items []datadogV2.HTTPLogItem) error {
		callCount++
		if callCount == 2 {
			return errors.New("simulated datadog outage")
		}
		return nil
	}

	summary, err := Run(cfg, sender, now)
	if err == nil {
		t.Fatal("expected an error from the failing second batch, got nil")
	}
	if summary.LogsSent != 1 {
		t.Fatalf("expected exactly 1 confirmed-sent log before the failure, got %+v", summary)
	}

	// The successfully sent ID must be persisted despite the overall error.
	store := NewSentStore(sentDir, now)
	sent, err := store.Load()
	if err != nil {
		t.Fatalf("unexpected error loading store: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("expected exactly 1 persisted sent id, got %v", sent)
	}
}

// TestRunNoNewLogsIsNotAnError checks an empty/absent log file is a clean no-op.
func TestRunNoNewLogsIsNotAnError(t *testing.T) {
	logDir := t.TempDir()
	sentDir := t.TempDir()
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)

	cfg := config.Config{
		LogDir: logDir, LogFileNamePattern: "app_20060102.log",
		LogsSentDir: sentDir, LogsSentRetention: 7 * 24 * time.Hour,
		BatchMaxItems: 10, BatchMaxBytes: 1 << 20,
	}

	called := false
	sender := func(items []datadogV2.HTTPLogItem) error {
		called = true
		return nil
	}

	summary, err := Run(cfg, sender, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.NewLogsFound != 0 || summary.LogsSent != 0 {
		t.Fatalf("expected an empty summary, got %+v", summary)
	}
	if called {
		t.Error("expected the sender to never be called when there is nothing to send")
	}
}
