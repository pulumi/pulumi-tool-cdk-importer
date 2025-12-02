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
  --import-file ./import.json \
  --local-stack-file ./capture-state \
  --keep-import-state
```

### Runtime mode

- Command: `runtime`
- Flags: `--stack` (repeatable), `--import-file` (optional), `--skip-create` (optional), `--verbose`
- Behavior: Uses the selected Pulumi stack and current working directory. If `--import-file` is provided, the tool imports into the selected stack and then exports state to produce a bulk import file (no local backend).

### Program mode

- Command: `program import`
- Flags: `--program-dir` (required), `--stack` (repeatable), `--import-file` (optional), `--skip-create` (optional), `--verbose`
- Behavior: Changes into `--program-dir`, runs against the selected stack. If `--import-file` is provided, the tool imports into the selected stack and exports state to produce a bulk import file.

### Program iterate (capture mode)

- Command: `program iterate`
- Flags: `--program-dir` (required), `--stack` (repeatable), `--import-file` (required), `--local-stack-file` (optional), `--keep-import-state` (optional), `--verbose`
- Behavior: Runs against a local file backend (optionally reusing `--local-stack-file`), forces `skip-create`, seeds the import file with `pulumi preview --import-file`, and writes the enriched import file. Use this for iterative capture without touching your real stack.

### Bulk import files

`--import-file` is supported in all commands:
- In `runtime` and `program import`, it writes an import spec after importing into the selected stack.
- In `program iterate`, it enables capture mode and is required.

The output includes:

- `nameTable` entries for every Pulumi resource, which lets `pulumi import --file` wire parents and providers correctly.
- Full AWS resource metadata (type, logical name, provider reference, component bit, provider version).
- Any property subsets captured during provider interception (useful for codegen hints).

The resulting `import.json` contains every CloudFormation resource Pulumi can map, with IDs populated wherever possible. Some resources with composite identifiers may show `<PLACEHOLDER>` IDs; fill those in manually before running `pulumi import --file import.json`. The importer also skips CDK metadata, nested stacks, and `Custom::*` resources, logging a summary so you can decide whether to handle them separately.

#### Partial import files and iterative workflows

**The tool will write an import file even if errors occur during execution.** This allows you to get a starting point (a partial import file) and iteratively improve it. The command will still exit with an error code, but the import file will contain whatever resources were successfully processed.

To build up your import file incrementally across multiple runs, use `program iterate` with `--local-stack-file` and `--keep-import-state` so the same local backend is reused.

#### Capture-mode options

- `--skip-create`: Suppresses the creation of the special CDK asset helper resources (buckets, ECR repos, IAM policy glue). This is automatically turned on for `program iterate`, but you can also enable it manually when experimenting.
- `--keep-import-state`: Keeps the temporary local backend directory so you can inspect the `Pulumi.dev.yaml`, exported stack files, or reuse them across multiple runs.
- `--local-stack-file`: Provides an explicit backend file path to reuse instead of letting the tool create a new temp directory. Combine this with `--keep-import-state` for deterministic CI runs.
- In `program iterate`, the tool first runs `pulumi preview --import-file <path>` in the program directory to generate a placeholder skeleton, then enriches it with captured/state data. The `--import-file` and backend paths are resolved relative to your invocation directory unless absolute.

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
