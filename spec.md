# Import-File Capture Mode (Option A)

This specification describes how to evolve the current importer so that it can emit a Pulumi bulk-import file **while still running through the proxied providers**. The key idea is to keep the existing interception pipeline (which already has every bit of information we need to derive primary IDs) but switch the behavior so we _record_ those IDs instead of issuing real `Create` RPCs.

## High-Level Flow

1. Users run `pulumi plugin run cdk-importer -- -stack <name> --import-file <path>`.
2. The CLI still calls `RunPulumiUpWithProxies`.
3. Interceptors compute primary IDs, issue `Read` calls to gather properties, but skip the real `Create`.
4. Every intercepted resource emits an entry (`type`, `name`, `logicalName`, `id`) into an in-memory collector.
5. After `s.Up()` completes, the collector serializes entries into `<path>` using the existing JSON writer.

## Detailed Tasks

- [x] **CLI plumbing**
  - ✅ Mode enum + `--import-file` wiring is in `main.go` and always flows through `RunPulumiUpWithProxies`.

- [x] **Collector data structure**
  - ✅ `Capture` + mutex-backed `CaptureCollector` with `Results()`/`Count()` helpers live in `internal/proxy/capture.go`.

- [x] **Proxy wiring**
  - ✅ `RunPulumiUpWithProxies` now accepts `RunOptions` (mode, path, collector), starts providers with those knobs, and writes the JSON file via `imports.WriteFile` after `s.Up()` in capture mode.
  - ⚠️ Stack-selection behavior matches the previous code path; still need to skip the Pulumi run entirely if no stack is selected (follow-up?).

- [x] **Interceptor behavior toggles**
  - ✅ Both AWS Classic + CCAPI interceptors take the collector + mode, compute IDs, append captures, short-circuit unsupported resource types, and only invoke real `Create` when running in `RunPulumi` mode.

- [ ] **Error handling & logging**
  - ✅ Unsupported resource types now bubble an error instead of silently creating them when capture mode is active.
  - ⏳ Need richer summary logging (counts, skipped resources, placeholders) once the collector feeds more metadata.

- [x] **File writer reuse**
  - ✅ Capture finalization reuses `internal/imports` to emit a `pulumi import --file` compatible JSON document (confirmed IDs only).

- [ ] **Tests**
  - ✅ Collector has unit coverage for dedupe + concurrency.
  - ⏳ Add proxy-level tests with fake providers to assert `Create` isn’t invoked in capture mode.
  - ⏳ Extend integration harness to cover capture mode (non-short only; compare JSON to golden file).

- [ ] **Docs**
  - ⏳ Update `README.md`/`AGENTS.md` with capture-mode instructions and stack-selection requirements.

## Open Questions

- How do we want to handle resources that currently fall back to `<PLACEHOLDER>` (e.g., CCAPI list failures)? We may still need a warning section in the output file.
- Should we dedupe component resources (URNs marked as components) or include them for completeness?

Document any decisions in this file so future sessions can adjust the checklist.
