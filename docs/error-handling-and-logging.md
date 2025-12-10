# Error Handling and Logging

This summarizes how errors and logs flow through the importer so humans (and future agents) know where to surface failures and why the event stream exists.

## Goals
- Avoid dumping Automation API stdout/stderr blobs directly to users.
- Emit concise CLI errors while preserving detailed diagnostics in logs.
- Keep resource import/failure counts accurate even when `pulumi up` fails.

## High-level flow
1) Validation/setup errors (e.g., missing stack selection, config writes, preview/import skeleton generation) bubble as-is; `cmd` trims Automation noise via `formatCLIError`.  
2) `RunPulumiUpWithProxies` runs `stack.Up` with `EventStreams`. All engine events go to an `upEventTracker`.  
3) On failure, we log a summarized view from the tracker and return a generic `operation failed` error. Capture mode still writes partial files when possible.  
4) On success, we return `nil` and compute import stats from the tracker’s counts.

## Event stream usage
- Why: Automation API includes stdout/stderr in `upErr`. We don’t want to parse that or show raw engine dumps. Event streams give structured diagnostics and resource failure context.  
- What’s tracked: resource registrations, create/import successes/failures, diagnostic errors (URN-specific and general), and engine-level errors.  
- Summary: the tracker builds a readable failure summary (URN + diagnostic). General diagnostics are included even if no resource op failed (e.g., program/init errors).

## Error return rules
- `upErr != nil`: log the tracker summary; return `errors.New("operation failed")`.  
- Capture mode: still logs and returns generic error, but writes partial import data if possible.  
- Non-Up automation calls: return wrapped error (may still carry stdout/stderr; `formatCLIError` trims).  
- Fallback: if no events were seen but a failure occurred, mark `resourcesFailedToImport = 1` so the completion log reflects failure.

## Logging visibility
- Default logs: info-level lines via `internal/logging` (friendly handler). Failure summaries are logged at info.  
- Verbose (`-v`): turns on Automation debug/flow logging to stdout.  
- CLI surface: `cmd/root.go` prints `formatCLIError(err)` once; Cobra usage/errors are silenced.

## Counters
- `resourcesImported` / `resourcesFailedToImport` come from the event tracker (`created` / `failedCreates`). Only if both are zero on failure do we fall back to `failed=1`.

## When adding new errors/logs
- Prefer emitting diagnostics through the event stream so they appear in the summary.  
- If you introduce new Automation API calls that can fail before events, ensure the returned error is meaningful and consider whether `formatCLIError` needs adjustment.  
- Keep CLI-facing errors concise; rely on logs for detail.
