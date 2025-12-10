# Repository Guidelines

This repo houses the experimental Pulumi tool that imports existing AWS CDK stacks into Pulumi state. Use this guide when contributing enhancements, fixing bugs, or extending the importer to cover additional resource types.

## Project Structure & Module Organization
- `main.go`, `identify.go`, and `generate.go` implement the CLI entry point, stack discovery logic, and code generation hooks.
- `internal/common`, `internal/lookups`, `internal/metadata`, and `internal/proxy` contain reusable helpers for resource mapping, AWS lookups, dynamic metadata, and proxying pulumi-cdk interactions.
- `integration/` hosts black-box tests; `integration/cdk-test` is a self-contained CDK app (TypeScript) used to validate end-to-end imports.
- `identify_test.go` plus package-level tests live alongside the Go sources; assets and fixtures stay co-located with the code that consumes them.

## Build, Test, and Development Commands
- `make build` (or `go build -o bin/pulumi-tool-cdk-importer`) produces the tool binary consumed by Pulumi.
- `make generate` triggers `go generate ./...` for any code-generation steps referenced in the tree.
- `make test` runs `go test -v -short -coverpkg=./... -coverprofile=coverage.txt ./...`, suitable for CI and fast local checks.
- `pulumi plugin run cdk-importer -- -stack <cf-stack>` exercises the compiled plugin against a CloudFormation stack; prefer pointing at disposable stacks while iterating.
  - `-import-file <path>` switches to capture mode, implies `-skip-create`, and writes the richer import manifest (see `spec.md`).
  - `-skip-create` can be set on its own to suppress the handful of special resources we otherwise create; the currently-selected stack is still used unless capture mode is active.
  - `-keep-import-state` skips deleting the temporary local backend when `-import-file` is used; `-local-stack-file` lets you re-use a specific backend file for debugging.

## Coding Style & Naming Conventions
- Target Go 1.24+ (`toolchain go1.24.1` in `go.mod`). Run `gofmt` and `goimports` before commits; keep imports grouped stdlib / external / internal.
- Favor small, focused packages within `internal/`; name files after the feature (`metadata_sync.go`, `lookups_route53.go`) and mirror that in tests (`*_test.go`).
- When logging, use structured errors (`fmt.Errorf`/`errors.Wrap`) and keep exported identifiers succinct yet descriptive.

## Testing Guidelines
- Unit tests should default to `t.Parallel()` where safe and stay short-mode friendly. Use table-driven tests in `*_test.go`.
- Run `go test ./...` before pushing; CI depends on collected coverage data in `coverage.txt`.
- Integration tests (`go test ./integration -run TestImport`) require AWS credentials, `AWS_REGION`, Node.js/Yarn, and CDK bootstrap permissions. They skip automatically under `-short`, so run without that flag before merging importer changes.

## Commit & Pull Request Guidelines
- Follow the existing history: concise, imperative subject plus optional scope (e.g., `Update dependency @pulumi/aws to v7.11.1 (#192)`). Reference issues or PR numbers in parentheses.
- PRs should describe the scenario, risks, and test evidence; attach `pulumi preview` or `go test` output when relevant, and include screenshots only if UI behavior changes (rare here).
- Keep diffs focused; separate dependency bumps from functional changes, and ensure `go.mod`/`go.sum` stay in sync.

## Security & Configuration Tips
- Never commit AWS credentials or Pulumi stack secrets. Use environment variables or Pulumi config (`pulumi config set aws:region us-west-2`) while testing.
- Integration runs create temporary CDK resources; clean them up with `cdk destroy --require-approval never --all` or via the provided test harness.
- When validating manual imports, export stacks to scratch directories (`pulumi stack export > /private/tmp/<name>.json`) listed under the writable roots above to avoid permission issues.

## Design Specs
- For ongoing work on the import-file capture mode (Option A), read `spec.md` at the repo root before making changes. Update the checklist there so future sessions know what remains.

## Documentation
- See `docs/README.md` for pointers to deep dives (error handling/logging with event streams, import preview mode, naming/IDs, CFN import background).
