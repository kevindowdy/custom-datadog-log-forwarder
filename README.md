# custom-datadog-log-forwarder

Forwards JSON log lines from a locally-rotated log file (such as the one
written by `fetch-open-vulnerabilities`) to Datadog Logs, on a schedule
(cron, Windows Task Scheduler, etc.), without ever modifying the source
log file.

## How it works

Each run:

1. Resolves today's log file from `LOG_DIR` + `LOG_FILE_NAME_PATTERN`.
2. Reads every complete (newline-terminated) line as JSON, expecting the
   fields `timestamp`, `level`, `log_id`, `message`, and `application`
   (this is exactly what `fetch-open-vulnerabilities`' `log_setup.py`
   writes). The `log_id` field must be a unique value per line — a UUID
   works well — since it's the sole dedup key.
3. Skips any line whose `log_id` was already recorded as sent earlier
   today (see "Dedup" below), and skips (with a warning, not a failure)
   any line that isn't valid JSON or is missing `log_id`.
4. Sends the remaining lines to Datadog in size-bounded batches.
5. Records every successfully-sent `log_id` in today's dedup tracking
   file, even if a later batch in the same run fails — so a retry never
   re-sends a log Datadog already has.

The log file is opened read-only and is never deleted, truncated, or
rotated by this application: it is safe to run on the same schedule as,
and against the same file that, another process keeps appending to.

## Configuration

All configuration is via environment variables (or a `.env` file next to
the binary; see `.env.example`).

| Variable                   | Required | Default        | Description |
|-----------------------------|----------|----------------|--------------|
| `DD_API_KEY`                | yes      | —              | Datadog API key. |
| `DD_SITE`                   | no       | `datadoghq.com`| Datadog site, e.g. `us5.datadoghq.com`. |
| `DD_API_BASE_URL`           | no       | —              | Overrides the Logs Submit endpoint entirely (e.g. to route through an internal proxy, or for local testing). |
| `LOG_DIR`                   | yes      | —              | Directory containing the source log files. Never reformatted. |
| `LOG_FILE_NAME_PATTERN`     | yes      | —              | Go `time.Format` layout for today's log file's *name only*, e.g. `saltminer_download_20060102.log`. |
| `LOGS_SENT_DIR`             | no       | `./logs_sent`  | Directory for the per-day dedup tracking files this app owns. |
| `LOGS_SENT_RETENTION_DAYS`  | no       | `7`            | How many days of tracking files to keep before pruning. |
| `DD_LOG_SOURCE`             | no       | (the log line's `application` field) | Overrides `ddsource` on forwarded logs. |
| `DD_LOG_SERVICE`            | no       | (the log line's `application` field) | Overrides `service` on forwarded logs. |
| `DD_LOG_TAGS`               | no       | —              | Comma-separated `ddtags` applied to every forwarded log. |
| `DD_LOG_HOSTNAME`           | no       | —              | Overrides `hostname` on forwarded logs. |
| `BATCH_MAX_ITEMS`           | no       | `400`          | Max log items per Datadog API call. |
| `BATCH_MAX_BYTES`           | no       | `3145728` (3 MiB) | Approximate max uncompressed payload size per batch. |

`LOG_FILE_NAME_PATTERN` is applied only to the file's base name, never to
`LOG_DIR`, so a directory name that happens to contain digits (a build
number, a timestamp, an IP) is never misinterpreted as part of the date
format.

### Dedup design notes

The dedup tracking file (`LOGS_SENT_DIR/logs_sent_YYYYMMDD.txt`) is this
application's own bookkeeping, scoped to one calendar day, and is the only
thing it ever prunes — the source log file is never touched. Because the
source application also rotates its own log file by date, re-reading the
whole of "today's" file on every run and filtering by `log_id` is cheap
and correct; there's no need to truncate or delete from it, and no risk of
losing data if this app and the source app are appending/reading at the
same time (only complete, newline-terminated lines are ever parsed).

## Setup Instructions

1. Ensure [Go](https://go.dev/dl/) 1.24 or later is installed.
2. Clone the repository:
   ```sh
   git clone https://github.com/kevindowdy/custom-datadog-log-forwarder.git
   cd custom-datadog-log-forwarder
   ```
3. Download dependencies:
   ```sh
   go mod download
   ```
4. Copy `.env.example` to `.env` and fill in your values.

## Run Instructions

Run the application directly:

```sh
go run ./src
```

Build a binary:

```sh
go build -o bin/forwarder ./src
./bin/forwarder
```

Schedule it (e.g. via cron) to run every 1-5 minutes; each run is a
single, complete, idempotent pass.

Run tests:

```sh
go test ./...
```

## Project Structure

```
src/
  main.go                     Binary entrypoint: wires config, forwarder, and the Datadog client together
  internal/config/            Environment-variable configuration loading
  internal/forwarder/         Log parsing, dedup tracking, batching, and the Run orchestration
  utilities/datadogclient/    Thin, testable wrapper around the Datadog Logs API
tests/                        Integration tests that build and run the actual binary
.github/                      CI/CD workflows and issue/PR templates
```

Unit tests live alongside the package they test (Go convention); `tests/`
holds the end-to-end test that builds and runs the real binary against a
local fake Datadog server.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Licensed under the terms in [LICENSE](LICENSE).
