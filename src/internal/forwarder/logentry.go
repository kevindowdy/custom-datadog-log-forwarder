// Package forwarder reads the application log file, dedupes lines against a
// per-day tracking file, and prepares/sends Datadog log items.
package forwarder

// Import encoding/json to decode each log line written by the source app.
import "encoding/json"

// Import fmt to build descriptive parse-error messages.
import "fmt"

// Import the Datadog API types for the outbound log item shape.
import "github.com/DataDog/datadog-api-client-go/v2/api/datadog"

// Import the Datadog v2 API types for the outbound log item shape.
import "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

// LogEntry mirrors one JSON log line written by the source application.
type LogEntry struct {
	// Timestamp is the human-readable time the source app recorded the line at.
	Timestamp string `json:"timestamp"`
	// Level is the log severity, e.g. "INFO" or "ERROR".
	Level string `json:"level"`
	// LogID is the unique identifier the forwarder dedupes on.
	LogID string `json:"log_id"`
	// Message is the human-readable log message.
	Message string `json:"message"`
	// Application is the name of the app that produced the line.
	Application string `json:"application"`
}

// ParsedLog pairs a Datadog log item with the source LogID used for dedup bookkeeping.
type ParsedLog struct {
	// Item is the Datadog log item ready to submit.
	Item datadogV2.HTTPLogItem
	// LogID is the identifier to record as sent once Item is delivered.
	LogID string
}

// parseLogLine decodes one raw JSON log line into a LogEntry.
func parseLogLine(line []byte) (LogEntry, error) {
	// Declare the entry that will hold the decoded fields.
	var entry LogEntry
	// Attempt to unmarshal the line as JSON into the entry.
	if err := json.Unmarshal(line, &entry); err != nil {
		// Wrap the decode error with context for the caller/log message.
		return LogEntry{}, fmt.Errorf("decode log line as JSON: %w", err)
	}
	// Require a non-empty log_id, since dedup depends entirely on it.
	if entry.LogID == "" {
		return LogEntry{}, fmt.Errorf("log line is missing a log_id field")
	}
	// Return the successfully decoded entry.
	return entry, nil
}

// tags holds the optional Datadog tag overrides applied to every forwarded log.
type tags struct {
	// source overrides ddsource; empty means "derive from the entry".
	source string
	// service overrides service; empty means "derive from the entry".
	service string
	// ddtags is the optional comma-separated ddtags string.
	ddtags string
	// hostname overrides hostname; empty means "leave unset".
	hostname string
}

// toHTTPLogItem converts a LogEntry into the Datadog API's log item shape.
func (e LogEntry) toHTTPLogItem(t tags) datadogV2.HTTPLogItem {
	// Determine the ddsource: an explicit override, or the entry's application name.
	source := t.source
	// Fall back to the entry's application field when no override is configured.
	if source == "" {
		source = e.Application
	}
	// Determine the service: an explicit override, or the entry's application name.
	service := t.service
	// Fall back to the entry's application field when no override is configured.
	if service == "" {
		service = e.Application
	}
	// Build the log item using the message as the primary body text.
	item := datadogV2.HTTPLogItem{
		Ddsource: datadog.PtrString(source),
		Service:  datadog.PtrString(service),
		Message:  e.Message,
		AdditionalProperties: map[string]interface{}{
			// Preserve the log_id on the forwarded log for cross-referencing in Datadog.
			"log_id": e.LogID,
			// Preserve the original level as a facet distinct from Datadog's own status.
			"level": e.Level,
			// Preserve the source application's own timestamp string as reported.
			"source_timestamp": e.Timestamp,
		},
	}
	// Attach ddtags only when the caller configured one, to avoid an empty tag string.
	if t.ddtags != "" {
		item.Ddtags = datadog.PtrString(t.ddtags)
	}
	// Attach hostname only when the caller configured one.
	if t.hostname != "" {
		item.Hostname = datadog.PtrString(t.hostname)
	}
	// Return the fully built log item.
	return item
}
