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

``` shell
❯ pulumi plugin run cdk-importer -- --help
Usage of pulumi-tool-cdk-importer:
  -import-file string
    	Write the Pulumi bulk import file produced from the CloudFormation stack to this path instead of mutating Pulumi state
  -keep-import-state
    	Retain the temporary local backend used when generating an import file (defaults to removing it)
  -local-stack-file string
    	Optional path to a local backend file to reuse while generating import files
  -skip-create
    	Skip creating special CDK asset helper resources (implied when -import-file is provided)
  -stack string
    	CloudFormation stack name to import
```

To migrate your existing CDK infrastructure to `pulumi-cdk`:

1. Follow instructions in the [pulumi/pulumi-cdk](https://github.com/pulumi/pulumi-cdk) repo to embed your CDK stacks in a Pulumi program

1. Instead of running `pulumi up`, run `pulumi plugin run cdk-importer -- -stack $CFStackName`. This will import the state of the 
  infrastructure defined by your CDK stack into Pulumi state. This operation is read-only (with the below exceptions) and should not modify any resources.

1. To verify that everything worked as expected, run `pulumi preview`. It should show no changes.

### CDK Integration

The tool can also directly integrate with a CDK application to generate the import file in a single step. This is done using the `-cdk-app` flag.

```shell
pulumi plugin run cdk-importer -- -cdk-app /path/to/cdk/app -stack my-stack
```

When `-cdk-app` is used, the tool performs the following steps:

1.  **Bundled cdk2pulumi**: It extracts an embedded version of `cdk2pulumi` (a tool that converts CDK apps to Pulumi programs).
2.  **Conversion**: It runs `cdk2pulumi` against the provided CDK application directory to generate a `Pulumi.yaml` and other necessary files in a temporary directory.
3.  **Import**: It then runs the import process using the generated Pulumi program as the source.

**Implied Flags**:

When using `-cdk-app`, the following flags are automatically set (unless you explicitly override them):

*   `-import-file`: Defaults to `import.json`. This enables "capture mode".
*   `-skip-create`: Defaults to `true`.
*   `-keep-import-state`: Defaults to `true`.
*   `-local-stack-file`: Defaults to `stack-state.json`.

This means a single command will convert your CDK app, capture the resource IDs from your deployed stack, and generate an `import.json` file ready for use with `pulumi import`.

### Generate a bulk import file

If you would rather produce a Pulumi [bulk import](https://www.pulumi.com/docs/iac/guides/migration/import/#bulk-import) spec (e.g., to pair with `pulumi import --file` or `--generate-code`), pass the `--import-file` flag:

```shell
pulumi plugin run cdk-importer -- -stack my-stack --import-file ./import.json
```

When `-import-file` is supplied, the tool spins up a throwaway local backend, runs against that stack without mutating your real state, and exports the results into an enriched import file. The output includes:

- `nameTable` entries for every Pulumi resource, which lets `pulumi import --file` wire parents and providers correctly.
- Full AWS resource metadata (type, logical name, provider reference, component bit, provider version).
- Any property subsets captured during provider interception (useful for codegen hints).

The resulting `import.json` contains every CloudFormation resource Pulumi can map, with IDs populated wherever possible. Some resources with composite identifiers may show `<PLACEHOLDER>` IDs; fill those in manually before running `pulumi import --file import.json`. The importer also skips CDK metadata, nested stacks, and `Custom::*` resources, logging a summary so you can decide whether to handle them separately.

#### Partial import files and iterative workflows

**The tool will write an import file even if errors occur during execution.** This allows you to get a starting point (a partial import file) and iteratively improve it. The command will still exit with an error code, but the import file will contain whatever resources were successfully processed.

To build up your import file incrementally across multiple runs:

1. Use `-local-stack-file /path/to/backend` to specify a persistent local backend
2. Use `-keep-import-state` to prevent cleanup of that backend after each run
3. Re-run the command as needed - each run will update the local stack state and regenerate the import file

Example iterative workflow:
```shell
# First run - may fail partway through
pulumi plugin run cdk-importer -- -stack my-stack \
  --import-file ./import.json \
  --local-stack-file ./capture-state \
  --keep-import-state

# Fix issues, then re-run with the same flags to continue building state
pulumi plugin run cdk-importer -- -stack my-stack \
  --import-file ./import.json \
  --local-stack-file ./capture-state \
  --keep-import-state
```

#### Capture-mode options

- `-skip-create`: Suppresses the creation of the special CDK asset helper resources (buckets, ECR repos, IAM policy glue). This is automatically turned on for capture mode, but you can also enable it manually when experimenting.
- `-keep-import-state`: Keeps the temporary local backend directory so you can inspect the `Pulumi.dev.yaml`, exported stack files, or reuse them across multiple runs.
- `-local-stack-file`: Provides an explicit backend file path to reuse instead of letting the tool create a new temp directory. Combine this with `-keep-import-state` for deterministic CI runs.

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
