// Package datadogclient standardizes this application's contract with the
// Datadog Logs API behind a small, testable interface.
package datadogclient

// Import context to thread request-scoped values (auth, site) to the SDK.
import "context"

// Import fmt to build descriptive error messages.
import "fmt"

// Import net/http to reference the SDK's raw response type in error messages.
import "net/http"

// Import os to read the optional base-URL override environment variable.
import "os"

// Import the Datadog SDK's shared client/configuration/context helpers.
import "github.com/DataDog/datadog-api-client-go/v2/api/datadog"

// Import the Datadog v2 Logs API types this client wraps.
import "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

// submitLogOperation is the Datadog SDK's internal name for the Logs Submit
// endpoint, used to key its per-operation server override map.
const submitLogOperation = "v2.LogsApi.SubmitLog"

// LogSubmitter is the subset of the Datadog Logs API this client depends on;
// satisfied by *datadogV2.LogsApi and by test fakes.
type LogSubmitter interface {
	// SubmitLog sends a batch of log items to Datadog.
	SubmitLog(ctx context.Context, body []datadogV2.HTTPLogItem, o ...datadogV2.SubmitLogOptionalParameters) (interface{}, *http.Response, error)
}

// Client sends batches of log items to Datadog through a LogSubmitter.
type Client struct {
	// submitter performs the actual API call.
	submitter LogSubmitter
	// ctx carries the DD-API-KEY/DD-APPLICATION-KEY/site values for every call.
	ctx context.Context
}

// New builds a Client authenticated from the DD_API_KEY/DD_APP_KEY/DD_SITE
// environment variables, which datadog.NewDefaultContext reads directly.
//
// If DD_API_BASE_URL is set, it replaces the Logs Submit endpoint's base URL
// entirely (bypassing DD_SITE for that one call). This exists so the
// forwarder can be pointed at an internal proxy in production, and so it can
// be exercised end to end in tests against a local fake server, without any
// real Datadog credentials or network access.
func New() *Client {
	// Build a context pre-populated with credentials/site from the environment.
	ctx := datadog.NewDefaultContext(context.Background())
	// Build the SDK configuration with its default settings (compression on, etc.).
	configuration := datadog.NewConfiguration()
	// Check for the optional base-URL override.
	if baseURL := os.Getenv("DD_API_BASE_URL"); baseURL != "" {
		// Point the Logs Submit operation's server list at the override URL.
		configuration.OperationServers[submitLogOperation] = datadog.ServerConfigurations{
			{URL: baseURL},
		}
	}
	// Build the underlying HTTP API client from that configuration.
	apiClient := datadog.NewAPIClient(configuration)
	// Return a Client wrapping the real Logs API and the credentialed context.
	return &Client{submitter: datadogV2.NewLogsApi(apiClient), ctx: ctx}
}

// NewWithSubmitter builds a Client around a caller-supplied LogSubmitter and
// context, letting tests substitute a fake with no network access.
func NewWithSubmitter(ctx context.Context, submitter LogSubmitter) *Client {
	// Return a Client wrapping the supplied submitter and context unchanged.
	return &Client{submitter: submitter, ctx: ctx}
}

// SubmitLogs sends one batch of log items to Datadog.
func (c *Client) SubmitLogs(logs []datadogV2.HTTPLogItem) error {
	// Do nothing when there is nothing to send.
	if len(logs) == 0 {
		return nil
	}
	// Call the Logs API with the default optional parameters.
	_, resp, err := c.submitter.SubmitLog(c.ctx, logs, *datadogV2.NewSubmitLogOptionalParameters())
	// Report a descriptive error when the call failed.
	if err != nil {
		// Default to a generic status description when no HTTP response is available.
		status := "no HTTP response"
		// Prefer the actual HTTP status line when one was returned.
		if resp != nil {
			status = resp.Status
		}
		// Wrap the underlying error with the batch size and HTTP status for context.
		return fmt.Errorf("submit %d log(s) to datadog: %w (%s)", len(logs), err, status)
	}
	// Report success.
	return nil
}
