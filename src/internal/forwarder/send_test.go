// Package forwarder tests for batching and sending log items.
package forwarder

// Import errors to build a sentinel failure for a fake sender.
import "errors"

// Import testing for the standard test framework.
import "testing"

// Import the Datadog v2 API types used to build test log items.
import "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

// buildParsedLogs returns n ParsedLog values with distinct, ordered LogIDs.
func buildParsedLogs(n int) []ParsedLog {
	// Preallocate the slice for n entries.
	logs := make([]ParsedLog, 0, n)
	// Build each entry with a distinct, predictable log ID for assertions.
	for i := 0; i < n; i++ {
		id := string(rune('a' + i))
		logs = append(logs, ParsedLog{
			Item:  datadogV2.HTTPLogItem{Message: "msg"},
			LogID: id,
		})
	}
	// Return the built slice.
	return logs
}

// TestSendLogsBatchesByMaxItems checks the item-count limit splits into multiple calls.
func TestSendLogsBatchesByMaxItems(t *testing.T) {
	// Build 5 logs to send with a batch size of 2.
	logs := buildParsedLogs(5)
	// Record the size of every batch the fake sender receives.
	var batchSizes []int
	sender := func(items []datadogV2.HTTPLogItem) error {
		batchSizes = append(batchSizes, len(items))
		return nil
	}
	// Send with a max item count of 2 and an effectively unlimited byte cap.
	sentIDs, err := SendLogs(sender, logs, 2, 1<<30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect batches of 2, 2, 1 for 5 total logs.
	if len(batchSizes) != 3 || batchSizes[0] != 2 || batchSizes[1] != 2 || batchSizes[2] != 1 {
		t.Errorf("unexpected batch sizes: %v", batchSizes)
	}
	// Expect every log ID to be reported as sent, in order.
	if len(sentIDs) != 5 {
		t.Errorf("expected 5 sent IDs, got %d: %v", len(sentIDs), sentIDs)
	}
}

// TestSendLogsBatchesByMaxBytes checks the byte-size limit splits into multiple calls.
func TestSendLogsBatchesByMaxBytes(t *testing.T) {
	// Build 3 logs whose serialized size is a few dozen bytes each.
	logs := buildParsedLogs(3)
	// Measure one item's actual size so the test isn't tied to a magic number.
	oneItemSize := estimateSize(logs[0].Item)
	// Record the size of every batch the fake sender receives.
	var batchSizes []int
	sender := func(items []datadogV2.HTTPLogItem) error {
		batchSizes = append(batchSizes, len(items))
		return nil
	}
	// Cap batches to just over one item's worth of bytes, forcing one item per batch.
	sentIDs, err := SendLogs(sender, logs, 1000, oneItemSize+1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect three separate single-item batches.
	if len(batchSizes) != 3 {
		t.Fatalf("expected 3 batches, got %d: %v", len(batchSizes), batchSizes)
	}
	for _, size := range batchSizes {
		if size != 1 {
			t.Errorf("expected each batch to contain 1 item, got %d", size)
		}
	}
	// Expect every log ID to be reported as sent.
	if len(sentIDs) != 3 {
		t.Errorf("expected 3 sent IDs, got %d: %v", len(sentIDs), sentIDs)
	}
}

// TestSendLogsReturnsPartialProgressOnFailure checks a later-batch failure still
// reports the IDs confirmed sent by every earlier, successful batch.
func TestSendLogsReturnsPartialProgressOnFailure(t *testing.T) {
	// Build 3 logs, sent one at a time so failure timing is precise.
	logs := buildParsedLogs(3)
	// Fail on the second call, simulating a mid-run API error.
	callCount := 0
	sender := func(items []datadogV2.HTTPLogItem) error {
		callCount++
		if callCount == 2 {
			return errors.New("simulated API failure")
		}
		return nil
	}
	// Send with a batch size of 1 so each log is its own call.
	sentIDs, err := SendLogs(sender, logs, 1, 1<<30)
	// An error must be returned once a batch fails.
	if err == nil {
		t.Fatal("expected an error from the failing batch, got nil")
	}
	// Only the first log's ID should be confirmed sent.
	if len(sentIDs) != 1 || sentIDs[0] != logs[0].LogID {
		t.Errorf("expected sentIDs = [%q], got %v", logs[0].LogID, sentIDs)
	}
}

// TestSendLogsNoopOnEmptyInput checks sending zero logs never calls the sender.
func TestSendLogsNoopOnEmptyInput(t *testing.T) {
	// Track whether the sender was ever invoked.
	called := false
	sender := func(items []datadogV2.HTTPLogItem) error {
		called = true
		return nil
	}
	// Send an empty slice of logs.
	sentIDs, err := SendLogs(sender, nil, 10, 1<<30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("expected the sender to never be called for empty input")
	}
	if len(sentIDs) != 0 {
		t.Errorf("expected no sent IDs, got %v", sentIDs)
	}
}
