# Import-File Capture Mode (Option A)

This document tracks the current plan for evolving capture mode so it can build
rich Pulumi import files without mutating user stacks.

## Goals

1. Allow operators to suppress creation of the “special” resources that do not
   map cleanly to CloudFormation logical IDs by introducing a `-skip-create`
   flag (implied whenever `-import-file` is used).
2. When `-import-file` is passed, run against a local file backend and emit an
   import file whose entries contain the newer metadata fields (`NameTable`,
   `Parent`, `Properties`, etc.). After the run, delete the temporary backend
   unless instructed otherwise. We now seed the import file with
   `pulumi preview --import-file` before enrichment.
3. Keep the existing capture collector so we can continue enriching import file
   entries with metadata that is not present in Pulumi state. Prioritize state
   data for wiring (names, parents, providers) and capture data for custom
   `Properties` hints.

## Detailed Plan (with Checkboxes)

### CLI and Flag Plumbing

- [x] Add `-skip-create` bool flag. Normal runs default to `false`; capture
      mode forces it to `true`.
- [x] Add `-keep-import-state` bool flag (default `false`). When set, capture
      mode leaves the local backend files intact after exiting.
- [x] Add `-local-stack-file` string flag so users can re-use a specific local
      backend file instead of a throwaway temp dir.
- [x] Ensure CLI parsing (now Cobra-based) enforces `-stack` and propagates the
      options through `proxy.RunOptions`.

### Stack/Backend Lifecycle for `-import-file`

- [x] When `-import-file` is not specified, continue using the currently
      selected stack (no backend changes).
- [x] When `-import-file` **is** specified:
  - [x] Create (or re-use) a local backend rooted at the path specified via
        `-local-stack-file` (or a temp dir if unspecified).
  - [x] Create a deterministic stack name (e.g., `capture-<stackRef>` or
        timestamp-based) within that backend using `auto.UpsertStackLocalSource`.
  - [x] After `Up()` completes, call `stack.Export()` to obtain state for import
        file generation.
  - [x] Delete the stack and backend directory unless `-keep-import-state` is
        set.

### Skip-Create Semantics

- [x] Extend `proxy.RunOptions` with a `SkipCreate` boolean so the interceptors
      know whether to bypass provider `Create` calls.
- [x] Update AWS interceptors to short-circuit the special resource types when
      `SkipCreate` is true: log the skip, record a `SkippedCapture`, and return a
      stub `CreateResponse` so the Pulumi engine considers the step successful.
- [x] Ensure normal runs (`SkipCreate` false) continue to invoke the real
      provider `Create` for those types to preserve current behavior.

### Import File Generation Enhancements

- [x] Introduce a function that merges data from two sources: (a) Pulumi state
      (exported deployment) and (b) the capture collector. State provides the
      authoritative resource set, URNs, parents/providers, and version info;
      capture metadata supplies property subsets or other hints.
- [x] Populate the new `NameTable` field by walking the exported deployment and
      mapping variable names to URNs.
- [x] For each AWS resource in state:
  - [x] Fill `Type`, `Name`, `LogicalName`, `Parent`, `Provider`, `Component`,
        and `Version` from state/metadata.
  - [x] Attach any `Properties` subset recorded via capture (if present).
  - [x] Skip non-AWS resources or Pulumi bookkeeping entries.
- [x] Continue calling `imports.WriteFile` to persist the enriched JSON to the
      user-supplied path.

### Testing & Docs

- [ ] Add unit tests that cover the new flag plumbing (parsing interactions,
      `-import-file` implying `-skip-create`, etc.).
- [ ] Add interceptor tests verifying that `SkipCreate` suppresses real provider
      calls yet still logs/skips entries.
- [ ] Extend existing capture-mode tests to exercise the backend export path
      and confirm the emitted JSON includes the new metadata fields.
- [x] Update `README.md` (and AGENTS.md if needed) to document the new flags,
      the optional persistent local stack file, and expectations for cleaning up
      temporary state.

## Open Questions / Follow-Ups

- Should we expose a knob for choosing the temporary stack name (useful in CI)?
- How should we surface skipped resources back to users—stdout log vs. summary
  block vs. both?
- Do we need to store additional metadata (e.g., ID shape hints) in the capture
  collector now, or can that wait until the schema consumers demand it?

Update this checklist as tasks land so future passes can see what remains.
