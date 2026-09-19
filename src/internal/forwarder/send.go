// Package forwarder: this file implements batching and sending log items to Datadog.
package forwarder

// Import encoding/json to estimate each log item's serialized size for batching.
import "encoding/json"

// Import fmt to build descriptive error messages.
import "fmt"

// Import the Datadog v2 API types for the batch payload shape.
import "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

// itemSender submits one batch of already-built Datadog log items.
//
// SendLogs depends on this function type rather than a concrete client so
// tests can inject a fake with no network access; datadogclient.Client's
// SubmitLogs method satisfies it directly.
type itemSender func(items []datadogV2.HTTPLogItem) error

// SendLogs submits parsed log entries to Datadog in size-bounded batches.
//
// It returns the log IDs that were confirmed sent, even when a later batch
// fails, so the caller can persist that partial progress and never resend a
// log that Datadog already accepted.
func SendLogs(send itemSender, logs []ParsedLog, maxItems, maxBytes int) ([]string, error) {
	// Track every log ID successfully confirmed sent so far.
	var sentIDs []string
	// Walk through the logs in batches bounded by count and approximate size.
	for start := 0; start < len(logs); {
		// Determine the end of this batch, respecting both count and byte limits.
		end := nextBatchEnd(logs, start, maxItems, maxBytes)
		// Build the slice of Datadog items for this batch.
		batch := make([]datadogV2.HTTPLogItem, 0, end-start)
		// Copy each item in the batch range.
		for _, parsed := range logs[start:end] {
			batch = append(batch, parsed.Item)
		}
		// Submit this batch to Datadog.
		if err := send(batch); err != nil {
			// Report the IDs confirmed before this failing batch, plus the error.
			return sentIDs, fmt.Errorf("send batch [%d:%d] of %d log(s): %w", start, end, len(logs), err)
		}
		// Record every log ID in this batch as confirmed sent.
		for _, parsed := range logs[start:end] {
			sentIDs = append(sentIDs, parsed.LogID)
		}
		// Advance to the next batch.
		start = end
	}
	// Every batch succeeded; report all sent IDs with no error.
	return sentIDs, nil
}

// nextBatchEnd returns the exclusive end index of the next batch starting at start.
func nextBatchEnd(logs []ParsedLog, start, maxItems, maxBytes int) int {
	// Track the running approximate byte size of the batch being built.
	size := 0
	// Extend the batch one item at a time until a limit would be exceeded.
	for i := start; i < len(logs); i++ {
		// Stop once the item count limit has been reached.
		if i-start >= maxItems {
			return i
		}
		// Estimate this item's serialized size to approximate the request payload size.
		itemSize := estimateSize(logs[i].Item)
		// Always include at least one item per batch, even if it alone exceeds maxBytes,
		// since Datadog still needs a chance to reject (or accept) it explicitly.
		if i > start && size+itemSize > maxBytes {
			return i
		}
		// Add this item's size to the running total and continue.
		size += itemSize
	}
	// The remaining items all fit in one final batch.
	return len(logs)
}

// estimateSize approximates an HTTPLogItem's serialized JSON size in bytes.
func estimateSize(item datadogV2.HTTPLogItem) int {
	// Marshal the item to JSON purely to measure its size.
	encoded, err := json.Marshal(item)
	// Fall back to a conservative fixed estimate if marshaling ever fails.
	if err != nil {
		return 1024
	}
	// Return the measured byte length.
	return len(encoded)
}
