// Package forwarder: this file orchestrates one end-to-end forwarder run.
package forwarder

// Import fmt to build descriptive error messages.
import "fmt"

// Import log to report warnings that don't stop the run.
import "log"

// Import time to timestamp the run and resolve today's files.
import "time"

// Import the Datadog v2 API types for the sender function signature.
import "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

// Import this module's config package for the resolved settings type.
import "github.com/kevindowdy/custom-datadog-log-forwarder/src/internal/config"

// Summary reports what one forwarder run accomplished, for logging/exit codes.
type Summary struct {
	// NewLogsFound is how many not-yet-sent log lines ParseLogs found.
	NewLogsFound int
	// LogsSent is how many of those were confirmed delivered to Datadog.
	LogsSent int
	// Warnings lists lines that were skipped because they couldn't be parsed.
	Warnings []string
}

// Run executes one full forwarder pass: prune stale tracking files, load
// today's dedup set, parse the source log file, send whatever is new, and
// persist every confirmed-sent log ID so it is never forwarded twice.
//
// The source log file is only ever opened for reading; Run never deletes,
// truncates, or otherwise modifies it, so it is always safe to run
// concurrently with whatever process keeps appending to that file.
func Run(cfg config.Config, send func(items []datadogV2.HTTPLogItem) error, now time.Time) (Summary, error) {
	// Prune stale dedup tracking files first; a failure here is logged but
	// never blocks forwarding, since it only affects future disk usage.
	if err := PruneOldSentFiles(cfg.LogsSentDir, cfg.LogsSentRetention, now); err != nil {
		log.Printf("warning: failed to prune old sent-log tracking files: %v", err)
	}

	// Resolve today's dedup tracking store.
	store := NewSentStore(cfg.LogsSentDir, now)
	// Load the set of log IDs already forwarded earlier today.
	alreadySent, err := store.Load()
	// A failure to load the dedup set is fatal: proceeding could double-send.
	if err != nil {
		return Summary{}, fmt.Errorf("load sent-log tracking store: %w", err)
	}

	// Parse the source log file for lines not yet in alreadySent.
	result, err := ParseLogs(cfg, alreadySent, now)
	// A failure to read the log file at all (beyond "not found") is fatal.
	if err != nil {
		return Summary{}, fmt.Errorf("parse log file: %w", err)
	}
	// Surface every malformed-line warning so an operator can investigate.
	for _, warning := range result.Warnings {
		log.Printf("warning: skipping unparseable log line: %s", warning)
	}

	// Build the summary now so it's populated even on an early return below.
	summary := Summary{NewLogsFound: len(result.Logs), Warnings: result.Warnings}

	// Nothing new to send; report success without touching the network.
	if len(result.Logs) == 0 {
		return summary, nil
	}

	// Send the new logs to Datadog in size-bounded batches.
	sentIDs, sendErr := SendLogs(send, result.Logs, cfg.BatchMaxItems, cfg.BatchMaxBytes)
	// Persist every confirmed-sent ID regardless of sendErr, so a partial
	// failure never causes an already-delivered log to be resent next run.
	if appendErr := store.Append(sentIDs); appendErr != nil {
		// Losing this bookkeeping risks duplicate sends next run, so it is
		// folded into the returned error rather than only logged.
		if sendErr != nil {
			return summary, fmt.Errorf("send failed (%v) and failed to persist %d confirmed-sent id(s): %w", sendErr, len(sentIDs), appendErr)
		}
		return summary, fmt.Errorf("failed to persist %d confirmed-sent id(s): %w", len(sentIDs), appendErr)
	}
	// Record how many logs were actually confirmed sent.
	summary.LogsSent = len(sentIDs)
	// Surface the send error, if any, now that its progress is safely persisted.
	if sendErr != nil {
		return summary, fmt.Errorf("send logs to datadog: %w", sendErr)
	}
	// Report full success.
	return summary, nil
}
