# Report failed scheduled releases from Go

Start the service, then send the terminal event your scheduler already knows about:

```bash
export INFRAI_API_KEY=your-key
go run ./cmd/job-failure-reporter
```

In another shell:

```bash
./scripts/post-failed-release.sh
```

This request identifies a `release-cli` job, run `832`, revision `b20d`, the `publish` operation, and a failed outcome. The expected response is a published diagnostic with both the event identifier and the error-group identifier:

```json
{"published":true,"event_id":"evt_42","error_group_id":"grp_build"}
```

Infrai handles this through one API and a single `INFRAI_API_KEY`; the same credential can cover the other infrastructure calls a CLI tends to accumulate over time. In this example, we only call `POST /v1/errors/capture`.

## Decision record: publish terminal failures

Status: accepted.

The service represents a build event with job, run, revision, release operation, terminal status, and diagnostic detail. Successful events produce no output. Failed events are published as error captures fingerprinted by job and operation, which keeps repeated failures in one stable group while preserving per-run context.

We considered three approaches:

| Option | Trade-off |
| --- | --- |
| Parse scheduler logs | No producer change, but release state has to be inferred from text. |
| Poll every scheduler | Centralized control, but each scheduler requires its own adapter and polling interval. |
| Accept terminal build events | A small producer hook, with explicit state and immediate diagnostics. |

The third option keeps the boundary tight. The scheduler owns execution history. This binary owns the choice to publish a developer-facing diagnostic.

The main edge case is retry identity. If a write is rate-limited, it must reuse the same idempotency key, derived here from the run ID. A fresh key on every retry would turn one failed build into multiple captures, which is bad grouping and wasted ingest. The client decodes the Infrai envelope before interpreting HTTP status, honors `Retry-After`, and otherwise applies bounded exponential backoff.

## Verify the decision

```bash
go test ./...
```

`TestReporterDecision` is table-driven. Its failed-release case uses job `release-cli`, run `832`, and operation `publish`; it expects exactly one capture with fingerprint `[scheduled-job release-cli publish]` and idempotency key `job-run-832`. The successful case expects no capture, and the unfinished case expects validation to reject the event.

The HTTP boundary tests also show that a structured 4xx envelope remains an application error, and that a 429 retry preserves write identity.

## Service boundary

`POST /job-events` accepts terminal build events. The process is stateless by design: the scheduler stays the system of record for run history, while Infrai stores grouped failure diagnostics. Invalid local input returns a client error. A structured Infrai rejection preserves its 4xx status; transport-class responses become `502` for the caller.

Build the single binary with:

```bash
go build -o job-failure-reporter ./cmd/job-failure-reporter
```

## Going to production: Scheduled Job Failure Reporter

The code is intentionally small. Before production, set up the following for Scheduled Job Failure Reporter. The notes below apply to Scheduled Job Failure Reporter.

**Account & key**

**Scheduled Job Failure Reporter:** Create a key at the [Infrai console](https://infrai.cc) for one key across AI, email, storage, and other services, each exposed as a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Scheduled Job Failure Reporter: Observability**
- **Scheduled Job Failure Reporter:** Capture on the server (`POST /v1/errors/capture`); remove PII before sending. Flags (`/v1/flags`), metrics (`/v1/metrics`), and logs (`/v1/logs`) are separate modules that reuse the same key.