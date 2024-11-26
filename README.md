# Pulumi CDK Importer Tool Plugin

Status: experimental

Assists migrating CDK-managed infrastructure to [pulumi-cdk](https://github.com/pulumi/pulumi-cdk).

## Installation

``` shell
pulumi plugin install tool cdk-importer
```

## Usage

``` shell
pulumi plugin run cdk-importer -- --help
...
  -stack string
        CloudFormation stack name
```

To migrate your existing CDK infrastructure to `pulumi-cdk`:

- Follow instructions in `pulumi-cdk` repo to transform your sources to Pulumi

- Instead of running `pulumi up`, run `pulumi plugin run cdk-importer -- -stack $CFStackName`. This will run `pulumi up`
  under the hood in a read-only mode that will not make any changes to your cloud but instead will import your
  infrastructure definitions into Pulumi state

- To verify that everything worked es expected, run `pulumi preview`. It should show no changes.
