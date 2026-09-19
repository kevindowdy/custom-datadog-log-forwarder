// Package tests holds integration tests that exercise the built forwarder
// binary as a whole, against a local fake Datadog server (never the real
// Datadog API), so they need no credentials and touch no external network.
package tests

// Import bytes to capture the binary's combined output.
import "bytes"

// Import encoding/json to decode the payload received by the fake server.
import "encoding/json"

// Import net/http to build the fake server's handler.
import "net/http"

// Import net/http/httptest to stand in for the real Datadog API over HTTP.
import "net/http/httptest"

// Import os to read the current environment for the child process.
import "os"

// Import os/exec to build and run the forwarder as a real subprocess.
import "os/exec"

// Import path/filepath to resolve the repo root and binary/log paths.
import "path/filepath"

// Import strings to assert on the binary's log output.
import "strings"

// Import sync to guard the fake server's shared state from its handler goroutine.
import "sync"

// Import testing for the standard test framework.
import "testing"

// Import time to compute today's log file name the same way the forwarder does.
import "time"

// buildForwarderBinary compiles the application's real entrypoint and
// returns the path to the resulting binary.
func buildForwarderBinary(t *testing.T) string {
	// Mark this as a helper so failures report the caller's line number.
	t.Helper()
	// Resolve the repository root, one directory above this test package.
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("failed to resolve repo root: %v", err)
	}
	// Choose an output path for the compiled binary inside a temp directory.
	binPath := filepath.Join(t.TempDir(), "forwarder")
	// Build the real main package, exactly as a deployment would.
	cmd := exec.Command("go", "build", "-o", binPath, "./src")
	cmd.Dir = repoRoot
	// Fail loudly, with the compiler's own output, on a build failure.
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build forwarder binary: %v\n%s", err, output)
	}
	// Return the path to the freshly built binary.
	return binPath
}

// fakeDatadogServer records every batch of logs it receives, standing in for
// the real Datadog Logs API for these tests.
type fakeDatadogServer struct {
	// server is the underlying httptest server.
	server *httptest.Server
	// mu guards batches against concurrent access from the handler goroutine.
	mu sync.Mutex
	// batches accumulates every decoded request body, in arrival order.
	batches []([]map[string]interface{})
}

// newFakeDatadogServer starts a fake server that always accepts submitted logs.
func newFakeDatadogServer() *fakeDatadogServer {
	// Construct the fake with no batches recorded yet.
	fake := &fakeDatadogServer{}
	// Start an httptest server backed by the fake's own handler method.
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	// Return the running fake.
	return fake
}

// handle decodes and records one incoming batch, then responds as Datadog would.
func (f *fakeDatadogServer) handle(w http.ResponseWriter, r *http.Request) {
	// Decode the request body as a batch of log items.
	var batch []map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&batch)
	// Record the batch under the mutex, since this runs on its own goroutine.
	f.mu.Lock()
	f.batches = append(f.batches, batch)
	f.mu.Unlock()
	// Respond with the same 202 Accepted status the real API returns on success.
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{}`))
}

// batchCount returns how many batches have been received so far.
func (f *fakeDatadogServer) batchCount() int {
	// Read the count under the mutex for a consistent snapshot.
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

// TestForwarderEndToEndDedupsAcrossRuns builds the real binary, runs it twice
// against a growing log file, and confirms: the first run forwards every new
// line, the dedup tracking file is what makes that possible, and a second
// run against the same file never re-sends a line already forwarded.
func TestForwarderEndToEndDedupsAcrossRuns(t *testing.T) {
	// Build the real forwarder binary once for this test.
	binPath := buildForwarderBinary(t)

	// Start the fake Datadog server and ensure it's closed when the test ends.
	fake := newFakeDatadogServer()
	defer fake.server.Close()

	// Set up isolated directories for the source log and dedup tracking files.
	logDir := t.TempDir()
	sentDir := t.TempDir()

	// Name today's log file exactly the way the forwarder will look for it.
	now := time.Now().UTC()
	logPath := filepath.Join(logDir, now.Format("app_20060102.log"))
	// Seed the log file with two well-formed, not-yet-sent entries.
	content := `{"timestamp":"t","level":"INFO","log_id":"id-1","message":"one","application":"app"}` + "\n" +
		`{"timestamp":"t","level":"INFO","log_id":"id-2","message":"two","application":"app"}` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to seed log file: %v", err)
	}

	// Build the environment the child process runs with: a fake API key, the
	// fake server as the Datadog endpoint, and the isolated test directories.
	env := append(os.Environ(),
		"DD_API_KEY=test-key",
		"DD_API_BASE_URL="+fake.server.URL,
		"LOG_DIR="+logDir,
		"LOG_FILE_NAME_PATTERN=app_20060102.log",
		"LOGS_SENT_DIR="+sentDir,
	)

	// runBinary runs the compiled forwarder once and returns its combined output.
	runBinary := func() string {
		cmd := exec.Command(binPath)
		cmd.Env = env
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("forwarder run failed: %v\n%s", err, out.String())
		}
		return out.String()
	}

	// Run the forwarder for the first time; it should find and send both lines.
	firstOutput := runBinary()
	if !strings.Contains(firstOutput, "found 2 new log(s), sent 2") {
		t.Errorf("unexpected first-run output: %s", firstOutput)
	}
	// Exactly one batch, containing both log items, should have reached the fake server.
	if fake.batchCount() != 1 {
		t.Fatalf("expected exactly 1 batch after the first run, got %d", fake.batchCount())
	}

	// Run the forwarder again against the same, unchanged log file.
	secondOutput := runBinary()
	if !strings.Contains(secondOutput, "found 0 new log(s), sent 0") {
		t.Errorf("unexpected second-run output: %s", secondOutput)
	}
	// No additional batch should have been sent: dedup must hold across runs.
	if fake.batchCount() != 1 {
		t.Fatalf("expected no additional batches on the second run, got %d total", fake.batchCount())
	}
}
