// Package forwarder: this file implements reading the source log file and
// turning its lines into log items not yet sent to Datadog.
package forwarder

// Import bufio to read the source log file efficiently, line by line.
import "bufio"

// Import fmt to build descriptive warning/error messages.
import "fmt"

// Import io to detect end-of-file while reading.
import "io"

// Import os for file I/O and existence checks.
import "os"

// Import path/filepath to join the plain log directory with today's file name.
import "path/filepath"

// Import time to resolve today's log file name and stamp warnings.
import "time"

// Import this module's config package for the resolved settings type.
import "github.com/kevindowdy/custom-datadog-log-forwarder/src/internal/config"

// ParseResult is everything ParseLogs produces from one pass over the log file.
type ParseResult struct {
	// Logs are the Datadog log items for lines not already recorded as sent.
	Logs []ParsedLog
	// Warnings describes lines that were skipped because they couldn't be used.
	Warnings []string
}

// ParseLogs resolves today's log file from cfg, reads it, and returns the log
// items for every complete line whose log_id isn't already in alreadySent.
//
// The source file is opened read-only and is never modified or truncated:
// the caller (the source application) may still be appending to it.
func ParseLogs(cfg config.Config, alreadySent map[string]struct{}, now time.Time) (ParseResult, error) {
	// Resolve today's log file name from the configured time-format pattern,
	// then join it with the plain log directory (which is never reformatted).
	logPath := filepath.Join(cfg.LogDir, now.Format(cfg.LogFileNamePattern))

	// Read every complete (newline-terminated) line from the log file.
	lines, err := readCompleteLines(logPath)
	// A missing log file simply means the source app hasn't written today's file yet.
	if err != nil {
		if os.IsNotExist(err) {
			return ParseResult{}, nil
		}
		// Any other read failure is unexpected and must surface to the caller.
		return ParseResult{}, fmt.Errorf("read log file %s: %w", logPath, err)
	}

	// Build the tag overrides once, shared by every converted log item.
	itemTags := tags{
		source:   cfg.DDSource,
		service:  cfg.DDService,
		ddtags:   cfg.DDTags,
		hostname: cfg.DDHostname,
	}

	// Prepare the result, sized optimistically to the number of lines read.
	result := ParseResult{Logs: make([]ParsedLog, 0, len(lines))}
	// Process each complete line independently, so one bad line never aborts the run.
	for i, line := range lines {
		// Skip lines that are blank once whitespace is discounted.
		if len(bytesTrimSpace(line)) == 0 {
			continue
		}
		// Attempt to decode the line as a log entry.
		entry, err := parseLogLine(line)
		// Record a warning and skip the line rather than failing the whole run.
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s line %d: %v", logPath, i+1, err))
			continue
		}
		// Skip entries whose log_id was already forwarded in an earlier run.
		if _, ok := alreadySent[entry.LogID]; ok {
			continue
		}
		// Convert the entry into a Datadog log item paired with its log_id.
		result.Logs = append(result.Logs, ParsedLog{
			Item:  entry.toHTTPLogItem(itemTags),
			LogID: entry.LogID,
		})
	}
	// Return the accumulated log items and any warnings.
	return result, nil
}

// readCompleteLines reads path and returns only its newline-terminated lines.
//
// The final chunk of a file is dropped whenever it lacks a trailing newline,
// since that means the writer (the source application) is still mid-write on
// that line; picking it up next run, once it's complete, is always correct.
func readCompleteLines(path string) ([][]byte, error) {
	// Open the log file for reading only; the forwarder never writes to it.
	file, err := os.Open(path)
	// Surface open failures (including "not exist") to the caller unchanged.
	if err != nil {
		return nil, err
	}
	// Ensure the file handle is released once this function returns.
	defer file.Close()

	// Wrap the file in a buffered reader for efficient line-oriented reads.
	reader := bufio.NewReader(file)
	// Accumulate the complete lines found so far.
	var lines [][]byte
	// Read until the underlying reader reports EOF.
	for {
		// Read up to and including the next newline character.
		line, err := reader.ReadBytes('\n')
		// A newline-terminated line was read successfully; record it without the delimiter.
		if err == nil {
			lines = append(lines, line[:len(line)-1])
			continue
		}
		// End of file was reached; any partial content in `line` is an in-progress
		// write and is intentionally discarded rather than treated as a full line.
		if err == io.EOF {
			break
		}
		// Any other error is unexpected and must surface to the caller.
		return nil, err
	}
	// Return every complete line found.
	return lines, nil
}

// bytesTrimSpace trims leading and trailing ASCII whitespace from a byte slice.
func bytesTrimSpace(b []byte) []byte {
	// Grow the start index past any leading whitespace bytes.
	start := 0
	for start < len(b) && isASCIISpace(b[start]) {
		start++
	}
	// Shrink the end index past any trailing whitespace bytes.
	end := len(b)
	for end > start && isASCIISpace(b[end-1]) {
		end--
	}
	// Return the trimmed slice.
	return b[start:end]
}

// isASCIISpace reports whether b is a common ASCII whitespace byte.
func isASCIISpace(b byte) bool {
	// Match space, tab, carriage return, newline, and other common blanks.
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\v' || b == '\f'
}
