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
  -stack string
    	CloudFormation stack name to import
```

To migrate your existing CDK infrastructure to `pulumi-cdk`:

1. Follow instructions in the [pulumi/pulumi-cdk](https://github.com/pulumi/pulumi-cdk) repo to embed your CDK stacks in a Pulumi program

1. Instead of running `pulumi up`, run `pulumi plugin run cdk-importer -- -stack $CFStackName`. This will import the state of the 
  infrastructure defined by your CDK stack into Pulumi state. This operation is read-only (with the below exceptions) and should not modify any resources.

1. To verify that everything worked as expected, run `pulumi preview`. It should show no changes.

### Generate a bulk import file

If you would rather produce a Pulumi [bulk import](https://www.pulumi.com/docs/iac/guides/migration/import/#bulk-import) spec (e.g., to pair with `pulumi import --file` or `--generate-code`), pass the `--import-file` flag:

```shell
pulumi plugin run cdk-importer -- -stack my-stack --import-file ./import.json
```

The resulting `import.json` contains every CloudFormation resource Pulumi can map, with IDs populated wherever possible. Some resources with composite identifiers may show `<PLACEHOLDER>` IDs; fill those in manually before running `pulumi import --file import.json`. The importer also skips CDK metadata, nested stacks, and `Custom::*` resources, logging a summary so you can decide whether to handle them separately.

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
