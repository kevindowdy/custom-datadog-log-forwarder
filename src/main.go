// Package main is the entrypoint for the application binary.
package main

// Import errors to check for a wrapped os.ErrNotExist with errors.Is.
import "errors"

// Import log to report the run's outcome and any fatal startup error.
import "log"

// Import os to control the process exit code on failure.
import "os"

// Import time to timestamp this run.
import "time"

// Import this module's config package to resolve settings from the environment.
import "github.com/kevindowdy/custom-datadog-log-forwarder/src/internal/config"

// Import this module's forwarder package, which does the actual work.
import "github.com/kevindowdy/custom-datadog-log-forwarder/src/internal/forwarder"

// Import this module's datadogclient package to submit logs to Datadog.
import "github.com/kevindowdy/custom-datadog-log-forwarder/src/utilities/datadogclient"

// Import godotenv to load DD_API_KEY and friends from a local .env file, if present.
import "github.com/joho/godotenv"

// main loads configuration, runs one forwarder pass, and reports the result.
//
// Intended to be invoked on a schedule (e.g. every 1-5 minutes) by cron,
// Windows Task Scheduler, or similar; each invocation is a single, complete
// pass over the source log file.
func main() {
	// Load a local .env file, if present; a missing file is not an error.
	// Use errors.Is (not os.IsNotExist) so a wrapped error is still matched.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("warning: failed to load .env file: %v", err)
	}

	// Resolve the forwarder's configuration from the environment.
	cfg, err := config.Load()
	// A configuration error means the forwarder cannot run at all.
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	// Build the Datadog client, authenticated from DD_API_KEY/DD_SITE.
	client := datadogclient.New()

	// Run one full forwarder pass: parse new log lines and send them to Datadog.
	summary, err := forwarder.Run(cfg, client.SubmitLogs, time.Now())
	// A run error means something needs operator attention (see logged warnings above).
	if err != nil {
		log.Fatalf("forwarder run failed: %v", err)
	}

	// Report a concise summary of what this run accomplished.
	log.Printf(
		"forwarder run complete: found %d new log(s), sent %d, skipped %d unparseable line(s)",
		summary.NewLogsFound, summary.LogsSent, len(summary.Warnings),
	)
}
