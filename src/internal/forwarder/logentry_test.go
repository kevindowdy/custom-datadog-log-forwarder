// Package forwarder tests for log line decoding and Datadog item conversion.
package forwarder

// Import testing for the standard test framework.
import "testing"

// TestParseLogLineSuccess checks a well-formed JSON line decodes correctly.
func TestParseLogLineSuccess(t *testing.T) {
	// Build a line matching what fetch-open-vulnerabilities' log_setup.py writes.
	line := []byte(`{"timestamp":"2026-09-19 10:00:00","level":"INFO","log_id":"abc-123","message":"hello","application":"fetch-open-vulnerabilities"}`)
	// Attempt to parse the line.
	entry, err := parseLogLine(line)
	// Fail the test if parsing unexpectedly returned an error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify every field was decoded correctly.
	if entry.LogID != "abc-123" || entry.Level != "INFO" || entry.Message != "hello" || entry.Application != "fetch-open-vulnerabilities" {
		t.Errorf("unexpected entry: %+v", entry)
	}
}

// TestParseLogLineRejectsInvalidJSON checks a malformed line is rejected, not panicked on.
func TestParseLogLineRejectsInvalidJSON(t *testing.T) {
	// Build a line that is not valid JSON at all.
	line := []byte(`not json`)
	// Attempt to parse the line and expect an error.
	if _, err := parseLogLine(line); err == nil {
		t.Fatal("expected an error for invalid JSON, got nil")
	}
}

// TestParseLogLineRequiresLogID checks a line without log_id is rejected.
func TestParseLogLineRequiresLogID(t *testing.T) {
	// Build a line that is valid JSON but omits log_id entirely.
	line := []byte(`{"timestamp":"2026-09-19 10:00:00","level":"INFO","message":"hello","application":"app"}`)
	// Attempt to parse the line and expect an error because log_id is required.
	if _, err := parseLogLine(line); err == nil {
		t.Fatal("expected an error for a missing log_id, got nil")
	}
}

// TestToHTTPLogItemDerivesFromApplication checks defaults come from the entry itself.
func TestToHTTPLogItemDerivesFromApplication(t *testing.T) {
	// Build an entry with no explicit tag overrides supplied.
	entry := LogEntry{
		Timestamp:   "2026-09-19 10:00:00",
		Level:       "ERROR",
		LogID:       "id-1",
		Message:     "boom",
		Application: "fetch-open-vulnerabilities",
	}
	// Convert the entry using empty tag overrides.
	item := entry.toHTTPLogItem(tags{})
	// Verify ddsource and service fell back to the application field.
	if item.GetDdsource() != "fetch-open-vulnerabilities" || item.GetService() != "fetch-open-vulnerabilities" {
		t.Errorf("expected ddsource/service derived from application, got ddsource=%q service=%q", item.GetDdsource(), item.GetService())
	}
	// Verify the message was carried through unchanged.
	if item.Message != "boom" {
		t.Errorf("Message = %q, want %q", item.Message, "boom")
	}
	// Verify the log_id was preserved as an additional property for cross-referencing.
	if item.AdditionalProperties["log_id"] != "id-1" {
		t.Errorf("AdditionalProperties[log_id] = %v, want %q", item.AdditionalProperties["log_id"], "id-1")
	}
	// Verify no ddtags/hostname were set, since none were configured.
	if item.Ddtags != nil || item.Hostname != nil {
		t.Errorf("expected no ddtags/hostname, got ddtags=%v hostname=%v", item.Ddtags, item.Hostname)
	}
}

// TestToHTTPLogItemAppliesOverrides checks explicit tag overrides win over the entry's fields.
func TestToHTTPLogItemAppliesOverrides(t *testing.T) {
	// Build a minimal entry.
	entry := LogEntry{LogID: "id-2", Message: "hi", Application: "app"}
	// Convert the entry with every override configured.
	item := entry.toHTTPLogItem(tags{source: "custom-source", service: "custom-service", ddtags: "env:prod", hostname: "host-1"})
	// Verify every overridden field took effect.
	if item.GetDdsource() != "custom-source" {
		t.Errorf("Ddsource = %q, want %q", item.GetDdsource(), "custom-source")
	}
	if item.GetService() != "custom-service" {
		t.Errorf("Service = %q, want %q", item.GetService(), "custom-service")
	}
	if item.GetDdtags() != "env:prod" {
		t.Errorf("Ddtags = %q, want %q", item.GetDdtags(), "env:prod")
	}
	if item.GetHostname() != "host-1" {
		t.Errorf("Hostname = %q, want %q", item.GetHostname(), "host-1")
	}
}
