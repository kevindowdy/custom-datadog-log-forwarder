// Package config loads the forwarder's settings from environment variables.
package config

// Import fmt to build descriptive error messages.
import "fmt"

// Import os to read environment variables.
import "os"

// Import strconv to parse integer environment variables.
import "strconv"

// Import time to express the retention window as a duration.
import "time"

// defaultLogsSentDir is used when LOGS_SENT_DIR is not set.
const defaultLogsSentDir = "./logs_sent"

// defaultLogsSentRetentionDays is how many days of dedup tracking files are kept.
const defaultLogsSentRetentionDays = 7

// defaultBatchMaxItems caps how many log items go in one Datadog API call.
const defaultBatchMaxItems = 400

// defaultBatchMaxBytes caps the approximate uncompressed payload size of one batch.
const defaultBatchMaxBytes = 3 * 1024 * 1024

// Config holds every setting the forwarder needs, resolved once at startup.
type Config struct {
	// LogDir is the plain directory containing the source application's log
	// files; it is never subject to time-format substitution.
	LogDir string
	// LogFileNamePattern is a time.Format layout (Go reference-time style,
	// e.g. "app_20060102.log") for today's log file's base name. Keeping the
	// format substitution scoped to just the file name — instead of the full
	// path — avoids silently corrupting a directory name that happens to
	// contain digits matching a reference-time token (e.g. "15", "01", "02").
	LogFileNamePattern string
	// LogsSentDir is the directory holding the per-day dedup tracking files.
	LogsSentDir string
	// LogsSentRetention is how long dedup tracking files are kept before pruning.
	LogsSentRetention time.Duration
	// DDSource overrides the ddsource tag on forwarded logs; empty means "derive it".
	DDSource string
	// DDService overrides the service tag on forwarded logs; empty means "derive it".
	DDService string
	// DDTags is an optional comma-separated ddtags string applied to forwarded logs.
	DDTags string
	// DDHostname overrides the hostname tag on forwarded logs; empty means "unset".
	DDHostname string
	// BatchMaxItems is the maximum number of log items sent in one API call.
	BatchMaxItems int
	// BatchMaxBytes is the approximate maximum uncompressed payload size per batch.
	BatchMaxBytes int
}

// Load reads and validates the forwarder's configuration from the environment.
func Load() (Config, error) {
	// Read the required log directory; there is no sane default for it.
	logDir := os.Getenv("LOG_DIR")
	// Fail fast if the directory is missing, since nothing else can proceed.
	if logDir == "" {
		return Config{}, fmt.Errorf("LOG_DIR environment variable is required")
	}

	// Read the required log file name pattern; there is no sane default for it.
	logFileNamePattern := os.Getenv("LOG_FILE_NAME_PATTERN")
	// Fail fast if the pattern is missing, since nothing else can proceed.
	if logFileNamePattern == "" {
		return Config{}, fmt.Errorf("LOG_FILE_NAME_PATTERN environment variable is required")
	}

	// Read the directory for dedup tracking files, defaulting if unset.
	logsSentDir := os.Getenv("LOGS_SENT_DIR")
	// Apply the default when the caller didn't set one.
	if logsSentDir == "" {
		logsSentDir = defaultLogsSentDir
	}

	// Parse how many days of tracking files to retain, defaulting if unset.
	retentionDays, err := getIntEnv("LOGS_SENT_RETENTION_DAYS", defaultLogsSentRetentionDays)
	// Surface a clear error if the value couldn't be parsed as an integer.
	if err != nil {
		return Config{}, err
	}
	// Reject a non-positive retention window since it would prune everything.
	if retentionDays <= 0 {
		return Config{}, fmt.Errorf("LOGS_SENT_RETENTION_DAYS must be a positive integer, got %d", retentionDays)
	}

	// Parse the maximum number of log items per batch, defaulting if unset.
	batchMaxItems, err := getIntEnv("BATCH_MAX_ITEMS", defaultBatchMaxItems)
	// Surface a clear error if the value couldn't be parsed as an integer.
	if err != nil {
		return Config{}, err
	}
	// Reject a non-positive batch size since it would never send anything.
	if batchMaxItems <= 0 {
		return Config{}, fmt.Errorf("BATCH_MAX_ITEMS must be a positive integer, got %d", batchMaxItems)
	}

	// Parse the maximum approximate batch payload size in bytes, defaulting if unset.
	batchMaxBytes, err := getIntEnv("BATCH_MAX_BYTES", defaultBatchMaxBytes)
	// Surface a clear error if the value couldn't be parsed as an integer.
	if err != nil {
		return Config{}, err
	}
	// Reject a non-positive byte cap since it would never send anything.
	if batchMaxBytes <= 0 {
		return Config{}, fmt.Errorf("BATCH_MAX_BYTES must be a positive integer, got %d", batchMaxBytes)
	}

	// Build and return the resolved configuration.
	return Config{
		LogDir:             logDir,
		LogFileNamePattern: logFileNamePattern,
		LogsSentDir:        logsSentDir,
		LogsSentRetention:  time.Duration(retentionDays) * 24 * time.Hour,
		DDSource:           os.Getenv("DD_LOG_SOURCE"),
		DDService:          os.Getenv("DD_LOG_SERVICE"),
		DDTags:             os.Getenv("DD_LOG_TAGS"),
		DDHostname:         os.Getenv("DD_LOG_HOSTNAME"),
		BatchMaxItems:      batchMaxItems,
		BatchMaxBytes:      batchMaxBytes,
	}, nil
}

// getIntEnv reads an integer environment variable, returning def when unset.
func getIntEnv(name string, def int) (int, error) {
	// Look up the raw string value for the given environment variable name.
	raw := os.Getenv(name)
	// Return the default immediately when the variable isn't set.
	if raw == "" {
		return def, nil
	}
	// Parse the raw string as a base-10 integer.
	value, err := strconv.Atoi(raw)
	// Wrap any parse failure with the offending variable name for context.
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, raw, err)
	}
	// Return the successfully parsed integer.
	return value, nil
}
