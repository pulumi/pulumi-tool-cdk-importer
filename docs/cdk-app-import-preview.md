# CDK App Import via `pulumi preview --import-file`

> Legacy note: the `--cdk-app` flow has been removed in favor of operating on existing Pulumi programs. This document is retained for historical context only.

Design notes for generating a complete `import.json` when `--cdk-app` is used by first asking Pulumi to emit the placeholder manifest, then enriching it with data we can derive.

## Goals

- Emit an import file that contains every resource reachable from the synthesized CDK app.
- Leverage `pulumi preview --import-file` to produce the base skeleton (schema-correct, Pulumi-maintained shape) instead of hand-rolling entries. This same preview-seeded skeleton is now used for any `-import-file` run, not just `--cdk-app`.
- Enrich the generated skeleton with any IDs/inputs we can infer from the CDK/CloudFormation template so users start with a mostly-complete manifest.
- Keep the workflow contained: temporary backend, no mutations to user stacks, and optional validation reruns.

## Assumptions / Pre-Reqs

- We can synthesize the CDK app to a Pulumi-compatible program (current `--cdk-app` flow).
- AWS credentials and config needed for preview are available (same requirements as existing preview).
- A stack backend is available (use local backend machinery we already have for capture mode).
- `pulumi preview --import-file <path>` will write the file with `<PLACEHOLDER>` IDs even when IDs are unknown; failures should be surfaced.

## Proposed Flow

1) **Synthesize** the CDK app into the temp Pulumi project (existing `cdk synth` + conversion step).  
2) **Backend prep**: ensure we use a local backend/stack (reuse `-local-stack-file`/`-keep-import-state` plumbing). Create or reuse the stack in that backend.  
3) **Initial preview run**: invoke `pulumi preview --import-file <path>` in the generated app directory. Capture any stderr/stdout for diagnostics. Stop early (and surface the error) if preview fails before writing the file.  
4) **Load skeleton**: read the generated `import.json` with `<PLACEHOLDER>` IDs. Treat it as the authoritative set of resources and schema fields.  
5) **Enrichment pass**: walk the synthesized CFN template and our existing metadata/lookups to fill:  
   - Resource IDs we can derive (ARNs, names) from template parameters/attributes, preferring captured IDs (the ones actually used for import) over exported state IDs.  
   - Parent/component relationships if they are missing or placeholder.  
   - Logical names and versions if needed.  
   - Leave `<PLACEHOLDER>` where unknown; optionally attach `Notes`/metadata explaining what is needed.  
6) **Write enriched file**: overwrite the same `import.json` (or a user-specified path) with the merged data.  
7) **Optional validation**: offer a second `pulumi preview --import-file <path>` run to confirm the enriched IDs are accepted (skip by default to keep runs short).  
8) **Cleanup**: delete the temp backend unless `-keep-import-state` is set.

## Data Mapping Notes

- Use the Pulumi-generated entries as the canonical resource list; if we cannot map a CFN resource to a Pulumi entry, log a warning.
- Prefer state-derived parent info from the preview output; fill gaps from our own mappings when placeholder/missing.
- Keep `NameTable` generation consistent with the capture-mode path; merge rather than replace if Pulumi adds entries.
- Preserve any Pulumi-added fields verbatim to avoid schema drift. Do not inject provider names; only retain versions.

## Error Handling

- If preview fails before emitting `import.json`, return a clear error with captured output (do not continue).
- If preview emits a partial file, treat that as failure unless we can detect completeness. Prefer fail-fast with actionable messaging (e.g., missing AWS config).
- Guard the enrichment step with schema validation to avoid writing malformed JSON.

## Testing Plan (minimum)

- Unit: ensure the new code path runs preview when `--cdk-app` is used without an existing import file and that we read/merge the emitted JSON. Use fakes/mocks to avoid real preview.  
- Unit: verify enrichment fills derivable IDs for known resource types and preserves `<PLACEHOLDER>` otherwise.  
- Integration (happy path): run against `integration/cdk-test` to confirm `import.json` is emitted and enriched.  
- Integration (failure): simulate missing AWS config to confirm we surface the preview error and do not write an empty/partial file.

## Open Questions

- Do we need a knob to skip the enrichment pass and leave the Pulumi-generated placeholders untouched?  
- Should we write the skeleton and enriched outputs to separate files (e.g., `import.skeleton.json` and `import.json`) for debugging?  
- Are there provider types where preview will not produce entries we expect (custom providers, policy packs)?
