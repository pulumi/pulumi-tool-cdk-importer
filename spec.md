# Lookup-ID CLI Command

Design for a new `lookup-id` command that returns the primary identifier for a CloudFormation resource using the existing Cloud Control API (CCAPI) lookup logic while accepting properties from user input (interactive prompts or flags).

## References
- Identifier behavior: `docs/ids.md`
- Background on property resolution challenges: `docs/why-cfn-import-is-hard.md`
- Metadata: `internal/metadata/schemas/primary-identifiers.json`, list handler requirements in `internal/metadata/schemas/pulumi-aws-native-metadata.json` (consumed by `metadata.ListHandlerRequiredProperties`)
- Overrides and strategies: `internal/metadata/ccapi.go`
- Lookup logic and retry/heuristics: `internal/lookups/ccapi.go`, tests in `internal/lookups/ccapi_test.go`

## Objectives
- Add a `lookup-id` CLI command that can resolve the primary ID for a resource by combining known properties (from stack/flags) with user-supplied values.
- Keep CCAPI lookup behavior intact: list → retry on missing property → ARN suffix heuristic → strategy overrides → primary identifier overrides.
- Provide clear outcomes: success (ID), needs-info (missing required filters), ambiguous (multiple matches), not-possible (no list handler/known service gap), or access-denied.
- Support interactive prompting and non-interactive `--input key=val`/`--no-interactive` flows for CI/batch.

## Current Behavior (what we are building on)
- `ccapiLookups.FindPrimaryResourceID` renders a resource model from Pulumi/CDK-resolved props and uses `FindResourceIdentifier` to list resources and match by physical ID prefix/suffix; retries on `InvalidRequestException` to extract missing properties; uses ARN suffix heuristic and strategy overrides.
- `metadata.ListHandlerRequiredProperties` currently auto-fills list handler required fields from the program-provided props, not from user input.
- Physical IDs come from the CF stack summaries in `Lookups.CfnStackResources`; the lookup flow assumes the stack was already discovered and props are known.

## Requirements & UX
- Inputs:
  - Target selection: `--stack <name> --logical-name <id>` (uses stack metadata) OR `--type <AWS::Type>` plus `--physical-id <value>` when stack info is unavailable.
  - Property injection: `--input Key=Value` (repeatable) to supply filter fields; `--input-json` could be a follow-up but out of scope unless easy.
  - Interaction: default prompts for missing required properties; `--no-interactive` to fail fast with a “needs-info” payload.
  - Region/profile should follow existing AWS config resolution (reuse `Lookups` wiring).
- Outputs:
  - Primary ID string on success (stdout).
  - Machine-consumable status (exit codes + structured message) for CI: success=0, needs-info/ambiguous/not-possible/access-denied use non-zero with JSON blob describing missing fields or ambiguity set.
  - Always echo which properties were used (source: stack/program vs user input vs derived).

## Design

### 1) Property Source Abstraction
- Introduce an interface (e.g., `PropertySource`) that can return values for a CFN/CCAPI property key and report its provenance.
- Implementations:
  - Program/stack source: wraps `CfnStackResource.Props` (current behavior).
  - User source: values parsed from `--input` or prompt responses.
- Composition: a resolver that checks user input first, then program props; exposes `RenderResourceModel(idParts, cfNameFunc)` and `GetMissingProps([]string)` helpers.
- Keep physical ID, strategy overrides, ARN heuristic logic unchanged; only swap the property retrieval/merge.

### 2) CLI Command: `lookup-id`
- Cobra command registered under root (`pulumi-tool-cdk-importer lookup-id`).
- Flags:
  - Target: `--stack`, `--logical-name`, `--type`, `--physical-id` (stack+logical preferred; type+physical fallback).
  - Input: `--input key=val` (repeatable), `--no-interactive`, `--region`/`--profile` passthrough (reuse existing config if possible), maybe `--format text|json` for output shape.
- Flow:
  1. Resolve target resource: from stack resources (if stack + logical provided) or from explicit type/physical ID pair.
  2. Pre-fill properties from known sources (stack/program props when available, plus user inputs).
  3. Pull required list handler fields from metadata (`ListHandlerRequiredProperties`) and map them to promptable fields.
  4. If interactive: prompt for missing required fields; if non-interactive: return needs-info with the missing keys.
  5. Run lookup using existing list/CCAPI logic and missing-property retry; match by physical ID suffix/heuristic; honor strategy overrides and primary identifier overrides.
  6. Disambiguate: if multiple matches remain, return “ambiguous” with the candidate identifiers.

### 3) List Handler Schema → Prompt Mapping
- Use `listHandlerSchema.required` from metadata to decide which fields to request.
- Basic types: strings/numbers/booleans/enums should be rendered into prompt text; default to string when type unknown.
- Nesting: if required field contains dots or camel-cased sub-objects, accept dotted key syntax in `--input` and prompts (e.g., `VpcConfiguration.SubnetIds[0]`), but defer deep validation—store as strings and rely on CCAPI errors for missing/invalid shapes.
- Provide help text when available from the schema properties (description, enum list).

### 4) Non-Interactive / CI Mode
- `--no-interactive` skips prompting. If required fields are missing, exit with needs-info and list the keys (and any enum/type hints).
- Ensure the command can run headless in pipelines without blocking.

### 5) Error Classes & Messaging
- NeedsInfo: missing required list handler fields (before or after retry).
- NotPossible: no list handler or CCAPI UnsupportedActionException; include service/type details and suggest manual override.
- Ambiguous: multiple matches; return the identifiers (or physical IDs) found.
- AccessDenied: propagate AWS access errors with a concise summary.
- Unknown/Other: bubble up existing error text; avoid masking current CCAPI retry behavior.

## Testing Plan
- Unit tests for the property source resolver (prioritization of user input, fallback to program props, dotted key handling).
- Extend `ccapi_test.go`-style coverage to:
  - Interactive path mocked via injected prompt reader (no actual TTY).
  - `--no-interactive` needs-info response when required list handler fields are absent.
  - Ambiguous match scenario (multiple identifiers returned).
  - Not-possible path (UnsupportedActionException) still returns placeholder per current behavior.
- CLI flag parsing tests: combinations of stack+logical vs type+physical, repeated `--input`, and format selection.

## Implementation Steps (execution order)
1) Introduce `PropertySource` abstraction and refactor `renderResourceModel`/`applyListHandlerResourceModel` to use it; keep lookup heuristics unchanged.
2) Add `lookup-id` Cobra command with target-selection and input flags; wire AWS config/init similar to existing runtime/program commands.
3) Build prompt/inputs module to satisfy required list handler fields and merge user-supplied values; add `--no-interactive` short-circuit.
4) Adapt lookup entrypoint to accept injected property source and target metadata (resource type, physical ID, logical name when available).
5) Implement output formatter for success/error classes (text + optional JSON).
6) Add tests mirroring `ccapi_test` coverage plus CLI and prompt behaviors; update docs/README to describe the new command and outcomes (success / needs-info / not-possible / ambiguous).

## Open Questions / Follow-Ups
- Should we support `--input-json` for structured properties and arrays?
- Do we want a `--cache` or `--max-pages` knob to bound long list operations?
- What exit codes should map to needs-info vs ambiguous vs access-denied (proposed: distinct non-zero values)?
- Should we surface which metadata (list handler required fields) drove prompts in the output for debugging?
