// Package forwarder tests for the per-day "already sent" tracking store.
package forwarder

// Import errors to check for a wrapped os.ErrNotExist with errors.Is.
import "errors"

// Import os for filesystem setup in tests.
import "os"

// Import path/filepath to build test file paths.
import "path/filepath"

// Import testing for the standard test framework.
import "testing"

// Import time to construct fixed "now" values for deterministic tests.
import "time"

// TestSentStoreLoadMissingFileReturnsEmptySet checks a first-ever run isn't an error.
func TestSentStoreLoadMissingFileReturnsEmptySet(t *testing.T) {
	// Use a fresh temp directory that has never had a tracking file written to it.
	dir := t.TempDir()
	// Build a store for a fixed date.
	store := NewSentStore(dir, time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC))
	// Load should succeed even though no file exists yet.
	sent, err := store.Load()
	// Fail if Load returned an unexpected error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect an empty set, not nil, so callers can index it safely.
	if len(sent) != 0 {
		t.Errorf("expected empty set, got %v", sent)
	}
}

// TestSentStoreAppendThenLoadRoundTrips checks appended IDs are visible after reload.
func TestSentStoreAppendThenLoadRoundTrips(t *testing.T) {
	// Use a fresh temp directory for this test's tracking file.
	dir := t.TempDir()
	// Build a store for a fixed date.
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	store := NewSentStore(dir, now)
	// Append two IDs as if they were just successfully forwarded.
	if err := store.Append([]string{"id-1", "id-2"}); err != nil {
		t.Fatalf("unexpected error appending: %v", err)
	}
	// Reload the store (a fresh value, simulating the next scheduled run).
	reloaded := NewSentStore(dir, now)
	sent, err := reloaded.Load()
	// Fail if Load returned an unexpected error.
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}
	// Both appended IDs must be present after reload.
	if _, ok := sent["id-1"]; !ok {
		t.Error("expected id-1 to be present after reload")
	}
	if _, ok := sent["id-2"]; !ok {
		t.Error("expected id-2 to be present after reload")
	}
}

// TestSentStoreAppendIsAdditive checks a second Append doesn't clobber earlier IDs.
func TestSentStoreAppendIsAdditive(t *testing.T) {
	// Use a fresh temp directory for this test's tracking file.
	dir := t.TempDir()
	// Build a store for a fixed date.
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	store := NewSentStore(dir, now)
	// Append IDs across two separate calls, simulating two forwarder runs in one day.
	if err := store.Append([]string{"id-1"}); err != nil {
		t.Fatalf("unexpected error on first append: %v", err)
	}
	if err := store.Append([]string{"id-2"}); err != nil {
		t.Fatalf("unexpected error on second append: %v", err)
	}
	// Load and verify both IDs from both calls survived.
	sent, err := store.Load()
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}
	if len(sent) != 2 {
		t.Errorf("expected 2 sent IDs, got %d: %v", len(sent), sent)
	}
}

// TestPruneOldSentFilesRemovesOnlyStaleFiles checks retention keeps recent files and this store's naming.
func TestPruneOldSentFilesRemovesOnlyStaleFiles(t *testing.T) {
	// Use a fresh temp directory to hold several tracking files.
	dir := t.TempDir()
	// Fix "now" so the test is deterministic regardless of when it runs.
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	// Create an old tracking file that should be pruned (10 days old, retention 7).
	oldPath := filepath.Join(dir, "logs_sent_20260909.txt")
	if err := os.WriteFile(oldPath, []byte("id-old\n"), 0o644); err != nil {
		t.Fatalf("failed to seed old file: %v", err)
	}
	// Create a recent tracking file that should be kept (today).
	recentPath := filepath.Join(dir, "logs_sent_20260919.txt")
	if err := os.WriteFile(recentPath, []byte("id-recent\n"), 0o644); err != nil {
		t.Fatalf("failed to seed recent file: %v", err)
	}
	// Create an unrelated file that should never be touched, matched or not.
	unrelatedPath := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(unrelatedPath, []byte("not a tracking file"), 0o644); err != nil {
		t.Fatalf("failed to seed unrelated file: %v", err)
	}
	// Prune with a 7-day retention window.
	if err := PruneOldSentFiles(dir, 7*24*time.Hour, now); err != nil {
		t.Fatalf("unexpected error pruning: %v", err)
	}
	// The old tracking file must be gone.
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected old tracking file to be pruned, stat err = %v", err)
	}
	// The recent tracking file must still exist.
	if _, err := os.Stat(recentPath); err != nil {
		t.Errorf("expected recent tracking file to survive, stat err = %v", err)
	}
	// The unrelated file must still exist, untouched.
	if _, err := os.Stat(unrelatedPath); err != nil {
		t.Errorf("expected unrelated file to survive, stat err = %v", err)
	}
}

// TestPruneOldSentFilesOnMissingDirIsNoop checks pruning a directory that doesn't exist yet is fine.
func TestPruneOldSentFilesOnMissingDirIsNoop(t *testing.T) {
	// Point at a directory that was never created.
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	// Pruning must not error just because there's nothing to prune yet.
	if err := PruneOldSentFiles(dir, 7*24*time.Hour, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
