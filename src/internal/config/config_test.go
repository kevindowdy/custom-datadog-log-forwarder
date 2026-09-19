// Package config tests validate environment-variable parsing and defaults.
package config

// Import testing for the standard test framework.
import "testing"

// Import time to assert on the resolved retention duration.
import "time"

// setRequiredEnv sets the two environment variables Load cannot proceed without.
func setRequiredEnv(t *testing.T) {
	// Set a plain log directory.
	t.Setenv("LOG_DIR", "/var/log/app")
	// Set a time-format file name pattern.
	t.Setenv("LOG_FILE_NAME_PATTERN", "app_20060102.log")
}

// TestLoadRequiresLogDir checks that a missing log directory is a hard error.
func TestLoadRequiresLogDir(t *testing.T) {
	// Set the file name pattern but leave the directory unset.
	t.Setenv("LOG_DIR", "")
	t.Setenv("LOG_FILE_NAME_PATTERN", "app_20060102.log")
	// Call Load and expect it to fail without the required variable.
	if _, err := Load(); err == nil {
		t.Fatal("expected an error when LOG_DIR is unset, got nil")
	}
}

// TestLoadRequiresLogFileNamePattern checks that a missing file name pattern is a hard error.
func TestLoadRequiresLogFileNamePattern(t *testing.T) {
	// Set the directory but leave the file name pattern unset.
	t.Setenv("LOG_DIR", "/var/log/app")
	t.Setenv("LOG_FILE_NAME_PATTERN", "")
	// Call Load and expect it to fail without the required variable.
	if _, err := Load(); err == nil {
		t.Fatal("expected an error when LOG_FILE_NAME_PATTERN is unset, got nil")
	}
}

// TestLoadAppliesDefaults checks default values when only the required vars are set.
func TestLoadAppliesDefaults(t *testing.T) {
	// Set only the required environment variables.
	setRequiredEnv(t)
	// Clear the optional variables so defaults are exercised.
	t.Setenv("LOGS_SENT_DIR", "")
	t.Setenv("LOGS_SENT_RETENTION_DAYS", "")
	t.Setenv("BATCH_MAX_ITEMS", "")
	t.Setenv("BATCH_MAX_BYTES", "")

	// Load the configuration under test.
	cfg, err := Load()
	// Fail immediately if Load returned an unexpected error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the log directory and file name pattern were passed through unchanged.
	if cfg.LogDir != "/var/log/app" {
		t.Errorf("LogDir = %q, want %q", cfg.LogDir, "/var/log/app")
	}
	if cfg.LogFileNamePattern != "app_20060102.log" {
		t.Errorf("LogFileNamePattern = %q, want %q", cfg.LogFileNamePattern, "app_20060102.log")
	}
	// Verify the default dedup directory was applied.
	if cfg.LogsSentDir != defaultLogsSentDir {
		t.Errorf("LogsSentDir = %q, want %q", cfg.LogsSentDir, defaultLogsSentDir)
	}
	// Verify the default retention window was applied.
	if cfg.LogsSentRetention != time.Duration(defaultLogsSentRetentionDays)*24*time.Hour {
		t.Errorf("LogsSentRetention = %v, want %v", cfg.LogsSentRetention, time.Duration(defaultLogsSentRetentionDays)*24*time.Hour)
	}
	// Verify the default batch item cap was applied.
	if cfg.BatchMaxItems != defaultBatchMaxItems {
		t.Errorf("BatchMaxItems = %d, want %d", cfg.BatchMaxItems, defaultBatchMaxItems)
	}
	// Verify the default batch byte cap was applied.
	if cfg.BatchMaxBytes != defaultBatchMaxBytes {
		t.Errorf("BatchMaxBytes = %d, want %d", cfg.BatchMaxBytes, defaultBatchMaxBytes)
	}
}

// TestLoadOverridesFromEnv checks that every optional variable can be overridden.
func TestLoadOverridesFromEnv(t *testing.T) {
	// Set the required variables.
	setRequiredEnv(t)
	// Override every optional variable with a distinct, recognizable value.
	t.Setenv("LOGS_SENT_DIR", "/tmp/sent")
	t.Setenv("LOGS_SENT_RETENTION_DAYS", "3")
	t.Setenv("BATCH_MAX_ITEMS", "10")
	t.Setenv("BATCH_MAX_BYTES", "1024")
	t.Setenv("DD_LOG_SOURCE", "custom-source")
	t.Setenv("DD_LOG_SERVICE", "custom-service")
	t.Setenv("DD_LOG_TAGS", "env:test")
	t.Setenv("DD_LOG_HOSTNAME", "test-host")

	// Load the configuration under test.
	cfg, err := Load()
	// Fail immediately if Load returned an unexpected error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify every overridden field was picked up.
	if cfg.LogsSentDir != "/tmp/sent" {
		t.Errorf("LogsSentDir = %q, want %q", cfg.LogsSentDir, "/tmp/sent")
	}
	// Verify the retention override converted to the correct duration.
	if cfg.LogsSentRetention != 3*24*time.Hour {
		t.Errorf("LogsSentRetention = %v, want %v", cfg.LogsSentRetention, 3*24*time.Hour)
	}
	// Verify the batch item override was picked up.
	if cfg.BatchMaxItems != 10 {
		t.Errorf("BatchMaxItems = %d, want %d", cfg.BatchMaxItems, 10)
	}
	// Verify the batch byte override was picked up.
	if cfg.BatchMaxBytes != 1024 {
		t.Errorf("BatchMaxBytes = %d, want %d", cfg.BatchMaxBytes, 1024)
	}
	// Verify the Datadog tag overrides were picked up.
	if cfg.DDSource != "custom-source" || cfg.DDService != "custom-service" || cfg.DDTags != "env:test" || cfg.DDHostname != "test-host" {
		t.Errorf("unexpected Datadog tag fields: %+v", cfg)
	}
}

// TestLoadRejectsInvalidIntegers checks that malformed integer variables error out.
func TestLoadRejectsInvalidIntegers(t *testing.T) {
	// Set the required variables.
	setRequiredEnv(t)
	// Set an unparsable value for one of the integer variables.
	t.Setenv("LOGS_SENT_RETENTION_DAYS", "not-a-number")
	// Call Load and expect it to fail because of the bad integer.
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a non-numeric LOGS_SENT_RETENTION_DAYS, got nil")
	}
}

// TestLoadRejectsNonPositiveIntegers checks that zero/negative values are rejected.
func TestLoadRejectsNonPositiveIntegers(t *testing.T) {
	// Set the required variables.
	setRequiredEnv(t)
	// Set a non-positive value that would otherwise silently misbehave.
	t.Setenv("BATCH_MAX_ITEMS", "0")
	// Call Load and expect it to fail because of the non-positive value.
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for BATCH_MAX_ITEMS=0, got nil")
	}
}
