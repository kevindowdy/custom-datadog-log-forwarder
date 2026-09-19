# Changelog

## [Unreleased]

### Added

- Implemented the forwarder: `internal/forwarder` parses the source log
  file (JSON lines with a `log_id` field), skips lines already recorded as
  sent in a per-day dedup tracking file, batches the rest by item
  count/byte size, and sends them to Datadog via `utilities/datadogclient`.
- Added `DD_API_BASE_URL` to override the Logs Submit endpoint, for
  routing through an internal proxy or for testing.
- Added an end-to-end integration test (`tests/integration_test.go`) that
  builds and runs the real binary twice against a local fake Datadog
  server, proving dedup holds across separate runs.

### Changed

- Renamed the module from the inherited `golang-template` placeholder to
  `github.com/kevindowdy/custom-datadog-log-forwarder`.

## [0.1.0] - 2026-09-18

### Added

- Initial project template: git commit -m "setup initial forwarder"
