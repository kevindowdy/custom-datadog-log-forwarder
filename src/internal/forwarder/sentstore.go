// Package forwarder: this file implements the per-day "already sent" tracking store.
package forwarder

// Import bufio to read the tracking file one ID per line.
import "bufio"

// Import fmt to build descriptive error messages.
import "fmt"

// Import os for file I/O.
import "os"

// Import path/filepath to join directories and match tracking file names.
import "path/filepath"

// Import strings to trim whitespace from tracking file lines.
import "strings"

// Import time to name/scope tracking files by day and to compute retention.
import "time"

// sentFilePrefix names the per-day dedup tracking files, e.g. "logs_sent_20060102.txt".
const sentFilePrefix = "logs_sent_"

// sentFileDateLayout is the Go reference-time layout embedded in tracking file names.
const sentFileDateLayout = "20060102"

// sentFileSuffix is the extension used for tracking files.
const sentFileSuffix = ".txt"

// SentStore tracks which log IDs have already been forwarded to Datadog "today".
type SentStore struct {
	// dir is the directory holding one tracking file per day.
	dir string
	// path is today's tracking file, resolved once at construction time.
	path string
}

// NewSentStore resolves today's tracking file path under dir, without touching disk yet.
func NewSentStore(dir string, now time.Time) SentStore {
	// Build today's tracking file name from the fixed prefix, date, and suffix.
	name := sentFilePrefix + now.Format(sentFileDateLayout) + sentFileSuffix
	// Return a store pointing at that file inside dir.
	return SentStore{dir: dir, path: filepath.Join(dir, name)}
}

// Load reads today's tracking file and returns the set of log IDs already sent.
func (s SentStore) Load() (map[string]struct{}, error) {
	// Start with an empty set; a missing file simply means nothing was sent yet today.
	sent := make(map[string]struct{})
	// Open today's tracking file for reading.
	file, err := os.Open(s.path)
	// Treat "file does not exist" as an empty, valid result rather than an error.
	if err != nil {
		if os.IsNotExist(err) {
			return sent, nil
		}
		// Any other error (permissions, I/O) is unexpected and must surface.
		return nil, fmt.Errorf("open sent-log tracking file %s: %w", s.path, err)
	}
	// Ensure the file handle is released once this function returns.
	defer file.Close()

	// Scan the file line by line, one log ID per line.
	scanner := bufio.NewScanner(file)
	// Read every line until EOF or a scan error.
	for scanner.Scan() {
		// Trim surrounding whitespace so stray blank lines don't become bogus IDs.
		id := strings.TrimSpace(scanner.Text())
		// Skip empty lines rather than recording them as a sent ID.
		if id == "" {
			continue
		}
		// Record the ID as already sent.
		sent[id] = struct{}{}
	}
	// Surface any error encountered while scanning.
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read sent-log tracking file %s: %w", s.path, err)
	}
	// Return the fully populated set of already-sent IDs.
	return sent, nil
}

// Append records newly sent log IDs by appending them to today's tracking file.
func (s SentStore) Append(ids []string) error {
	// Do nothing when there is nothing to record.
	if len(ids) == 0 {
		return nil
	}
	// Ensure the tracking directory exists before writing into it.
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create sent-log tracking directory %s: %w", s.dir, err)
	}
	// Open today's tracking file for appending, creating it if it doesn't exist yet.
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	// Surface any failure to open the file for writing.
	if err != nil {
		return fmt.Errorf("open sent-log tracking file %s for append: %w", s.path, err)
	}
	// Ensure the file handle is released once this function returns.
	defer file.Close()

	// Buffer the writes so a large batch of IDs results in few syscalls.
	writer := bufio.NewWriter(file)
	// Write each ID on its own line.
	for _, id := range ids {
		if _, err := writer.WriteString(id + "\n"); err != nil {
			return fmt.Errorf("write sent-log id to %s: %w", s.path, err)
		}
	}
	// Flush the buffered writes to disk.
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush sent-log tracking file %s: %w", s.path, err)
	}
	// Report success.
	return nil
}

// PruneOldSentFiles deletes tracking files older than retention, relative to now.
//
// This only ever removes the forwarder's own bookkeeping files under dir; it
// never touches the source application's log file.
func PruneOldSentFiles(dir string, retention time.Duration, now time.Time) error {
	// List the tracking directory's entries; a missing directory means nothing to prune.
	entries, err := os.ReadDir(dir)
	// Treat a missing directory as "nothing to prune" rather than an error.
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read sent-log tracking directory %s: %w", dir, err)
	}
	// Compute the cutoff date: files strictly older than this are pruned.
	cutoff := now.Add(-retention)
	// Inspect each entry in the directory.
	for _, entry := range entries {
		// Skip subdirectories; tracking files are always flat files.
		if entry.IsDir() {
			continue
		}
		// Extract the date embedded in the file name, skipping names that don't match.
		fileDate, ok := parseSentFileDate(entry.Name())
		// Skip files that don't match this store's naming convention.
		if !ok {
			continue
		}
		// Keep the file when its date is on or after the cutoff.
		if !fileDate.Before(cutoff) {
			continue
		}
		// Remove the tracking file; a delete race with another process is not fatal.
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove old sent-log tracking file %s: %w", entry.Name(), err)
		}
	}
	// Report success.
	return nil
}

// parseSentFileDate extracts the date embedded in a tracking file name, if any.
func parseSentFileDate(name string) (time.Time, bool) {
	// Require the fixed prefix and suffix to identify a tracking file.
	if !strings.HasPrefix(name, sentFilePrefix) || !strings.HasSuffix(name, sentFileSuffix) {
		return time.Time{}, false
	}
	// Extract the date portion between the prefix and suffix.
	dateStr := strings.TrimSuffix(strings.TrimPrefix(name, sentFilePrefix), sentFileSuffix)
	// Parse the date portion using the same layout the files are named with.
	parsed, err := time.Parse(sentFileDateLayout, dateStr)
	// Report failure when the date portion doesn't parse cleanly.
	if err != nil {
		return time.Time{}, false
	}
	// Report the successfully parsed date.
	return parsed, true
}
