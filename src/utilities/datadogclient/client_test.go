// Package datadogclient tests exercise Client against a fake LogSubmitter,
// so no network access or real Datadog credentials are required.
package datadogclient

// Import context for the SubmitLog signature.
import "context"

// Import encoding/json to inspect the payload received by a fake HTTP server.
import "encoding/json"

// Import errors to build a sentinel failure for the fake submitter.
import "errors"

// Import net/http for the SDK's response type.
import "net/http"

// Import net/http/httptest to stand in for the real Datadog API over HTTP.
import "net/http/httptest"

// Import testing for the standard test framework.
import "testing"

// Import the Datadog v2 API types used by the fake submitter.
import "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

// fakeSubmitter records every batch it receives and returns a canned result.
type fakeSubmitter struct {
	// received accumulates every batch passed to SubmitLog, in call order.
	received [][]datadogV2.HTTPLogItem
	// err is returned from every call, simulating an API failure when non-nil.
	err error
	// resp is returned alongside err, simulating the SDK's raw HTTP response.
	resp *http.Response
}

// SubmitLog implements datadogclient.LogSubmitter for tests.
func (f *fakeSubmitter) SubmitLog(_ context.Context, body []datadogV2.HTTPLogItem, _ ...datadogV2.SubmitLogOptionalParameters) (interface{}, *http.Response, error) {
	// Record this call's batch for later assertions.
	f.received = append(f.received, body)
	// Return the canned result configured on the fake.
	return nil, f.resp, f.err
}

// TestSubmitLogsSendsBatch checks a successful call passes the batch through unchanged.
func TestSubmitLogsSendsBatch(t *testing.T) {
	// Build a fake submitter that always succeeds.
	fake := &fakeSubmitter{}
	// Build a client around the fake submitter.
	client := NewWithSubmitter(context.Background(), fake)
	// Submit a single log item.
	logs := []datadogV2.HTTPLogItem{{Message: "hello"}}
	// Expect no error from a successful submission.
	if err := client.SubmitLogs(logs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect exactly one call carrying exactly one log item.
	if len(fake.received) != 1 || len(fake.received[0]) != 1 {
		t.Fatalf("expected 1 batch of 1 log, got %+v", fake.received)
	}
}

// TestSubmitLogsNoopOnEmpty checks an empty batch never calls the API.
func TestSubmitLogsNoopOnEmpty(t *testing.T) {
	// Build a fake submitter that always succeeds.
	fake := &fakeSubmitter{}
	// Build a client around the fake submitter.
	client := NewWithSubmitter(context.Background(), fake)
	// Submit a nil/empty slice of logs.
	if err := client.SubmitLogs(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect the fake was never invoked.
	if len(fake.received) != 0 {
		t.Fatalf("expected no API call for empty logs, got %d calls", len(fake.received))
	}
}

// TestSubmitLogsWrapsError checks a failing call returns a descriptive error.
func TestSubmitLogsWrapsError(t *testing.T) {
	// Build a fake submitter that always fails with a server error response.
	fake := &fakeSubmitter{err: errors.New("boom"), resp: &http.Response{Status: "500 Internal Server Error"}}
	// Build a client around the fake submitter.
	client := NewWithSubmitter(context.Background(), fake)
	// Submit a single log item and expect an error back.
	err := client.SubmitLogs([]datadogV2.HTTPLogItem{{Message: "hi"}})
	// Fail the test if no error was returned.
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestNewHonorsBaseURLOverride checks New() routes SubmitLogs through
// DD_API_BASE_URL when set, using a real HTTP round trip against a local
// fake server rather than the real Datadog API.
func TestNewHonorsBaseURLOverride(t *testing.T) {
	// Track whether the fake server received a request, and what it contained.
	var receivedPath string
	var receivedBody []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	// Point the client at the fake server instead of the real Datadog API.
	t.Setenv("DD_API_KEY", "test-key")
	t.Setenv("DD_API_BASE_URL", server.URL)

	client := New()
	err := client.SubmitLogs([]datadogV2.HTTPLogItem{{Message: "hello from test"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The SDK appends the fixed /api/v2/logs path to whatever base URL is configured.
	if receivedPath != "/api/v2/logs" {
		t.Errorf("path = %q, want %q", receivedPath, "/api/v2/logs")
	}
	if len(receivedBody) != 1 || receivedBody[0]["message"] != "hello from test" {
		t.Errorf("unexpected body received by fake server: %+v", receivedBody)
	}
}
