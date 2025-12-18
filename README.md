# Pulumi CDK Importer Tool Plugin

Assists migrating CDK-managed infrastructure to [pulumi-cdk](https://github.com/pulumi/pulumi-cdk).

> [!CAUTION]
> This is currently an **experimental** tool. It may not be able to migrate all CDK stacks and may have undocumented behaviors and/or bugs.
> Users should carefully test the behavior before using on production stacks.

## Installation

``` shell
pulumi plugin install tool cdk-importer
```

## Usage

Run `pulumi plugin run cdk-importer -- --help` to see the command tree. The CLI now uses Cobra with two primary flows:

- `runtime`: Import from the pulumi-cdk runtime program in the current directory (creates allowed). Use this when you already have a Pulumi program embedding your CDK app.
- `program import`: Import into the selected stack using an existing Pulumi program located elsewhere.
- `program iterate`: Capture mode against an existing Pulumi program using a local backend and import file for iterative refinement.

Examples:

```shell
# Runtime mode in the current directory (imports into selected stack)
pulumi plugin run cdk-importer -- runtime --stack Stack1

# Runtime mode and also emit an import file based on the selected stack
pulumi plugin run cdk-importer -- runtime --stack Stack1 --import-file ./import.json

# Program mode: import into selected stack using a generated Pulumi program
pulumi plugin run cdk-importer -- program import --program-dir ./generated --stack Stack1

# Program mode, iterative capture with local backend + import file
pulumi plugin run cdk-importer -- program iterate \
  --program-dir ./generated \
  --stack Stack1 \
  --import-file ./import.json
```

### Runtime mode

- Command: `runtime`
- Flags: `--stack` (repeatable), `--import-file` (optional, defaults to `import.json` when provided without a value), `--skip-create` (optional), `-v/--verbose`, `--debug` (importer debug logs only)
- Behavior: Uses the selected Pulumi stack and current working directory. With `--import-file`, the tool never runs `pulumi preview`; it reuses the file if it already exists, otherwise it seeds an import file skeleton from engine resource registration events and writes a file containing only the resources that failed during the run.

### Program mode

- Command: `program import`
- Flags: `--program-dir` (required), `--stack` (repeatable), `--import-file` (optional, defaults to `import.json` when provided without a value), `-v/--verbose`, `--debug`
- Behavior: Changes into `--program-dir`, forces `skip-create` (no asset helper creation), runs against the selected stack. With `--import-file`, the tool never runs `pulumi preview`; it reuses the file if it already exists, otherwise it seeds an import file skeleton from engine resource registration events and writes a file containing only the resources that failed during the run.

### Program iterate (capture mode)

- Command: `program iterate`
- Flags: `--program-dir` (required), `--stack` (repeatable), `--import-file` (optional, defaults to `import.json`), `-v/--verbose`, `--debug`
- Behavior: Runs against a persistent local file backend at `.pulumi/import-state.json` (relative to your invocation dir), forces `skip-create`, and always writes the enriched import file (partial on failure). The file is seeded from engine resource registration events (and merged with an existing `import.json` if present), so no `pulumi preview` is required. Use this for iterative capture without touching your real stack; the local backend is kept for reuse between runs.

### Bulk import files

`--import-file` is supported in all commands. Pass `--import-file` with no value to write `import.json`, or supply a path to choose a filename. `program iterate` also defaults to `import.json` when the flag is omitted entirely.
- `runtime` and `program import` run `pulumi up` against the selected stack, then write an import file that only includes resources that failed during the run.
- `program iterate` runs `pulumi up` against the local backend and writes the full enriched import spec (partial on failure) for iterative capture.

The output includes:

- Full AWS resource metadata (type, logical name, component bit, provider version).
- Any property subsets captured during provider interception (useful for codegen hints).

For capture/iterate flows, the resulting `import.json` contains every resource observed during the run, with IDs populated wherever possible. When using `runtime` or `program import` with `--import-file`, the written file is trimmed down to only the resources that failed so you can fill them in (or adjust the program) and retry import. The importer also skips CDK metadata, nested stacks, and `Custom::*` resources, logging a summary so you can decide whether to handle them separately.

#### Partial import files and iterative workflows

**The tool will write an import file even if errors occur during execution.** This allows you to get a starting point (a partial import file) and iteratively improve it. The command will still exit with an error code, but the import file will contain whatever resources were successfully processed.

To build up your import file incrementally across multiple runs, use `program iterate`; it keeps the local backend at `.pulumi/import-state.json` for reuse.

#### Capture-mode options

- `--skip-create`: Suppresses the creation of the special CDK asset helper resources (buckets, ECR repos, IAM policy glue). This is enforced for `program import` and `program iterate`, and can be enabled manually in `runtime` if you want to avoid creating those helpers.
- Local backend: `program iterate` always uses and retains a local backend rooted at `.pulumi/import-state.json`; delete that directory if you want a fresh capture.
- When an import file is requested, the tool reuses the existing file (if present) as an input skeleton, otherwise it seeds a skeleton from engine resource registration events, then enriches it with captured/state data. The `--import-file` paths are resolved relative to your invocation directory unless absolute.

### Unsupported Resources

There are some resources that the tool is unable to import. Some of these
resources are related to assets and will be created by the tool.

**Resources that will be created**

- Resources required by CDK File Assets
  - `aws:s3/bucketObjectv2:BucketObjectv2`
  - `aws:s3/bucketV2:BucketV2`
  - `aws:s3/bucketLifecycleConfigurationV2:BucketLifecycleConfigurationV2`
  - `aws:s3/bucketServerSideEncryptionConfigurationV2:BucketServerSideEncryptionConfigurationV2`
  - `aws:s3/bucketPolicy:BucketPolicy`
  - `aws:s3/bucketVersioningV2:BucketVersioningV2`
- Resources required by CDK Image Assets
  - `aws:ecr/repository:Repository`
  - `aws:ecr/lifecyclePolicy:LifecyclePolicy`

**Due to [pulumi/pulumi-cdk#293](https://github.com/pulumi/pulumi-cdk/issues/293)**
- `aws:iam/policy:Policy`
- `aws:iam/rolePolicyAttachment:RolePolicyAttachment`

**Resources that will not be imported**

- CFN Custom Resources (`aws-native:cloudformation:CustomResourceEmulator`).
    - Upvote [#6](https://github.com/pulumi/pulumi-tool-cdk-importer/issues/6) if this affects you
