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
Usage of /Users/mjeffryes/.pulumi/plugins/tool-cdk-importer-v0.0.1-alpha.3/pulumi-tool-cdk-importer:
  -alsologtostderr
    	log to standard error as well as files
  -classic-bin string
    	Location to the aws classic bin
  -log_backtrace_at value
    	when logging hits line file:N, emit a stack trace
  -log_dir string
    	If non-empty, write log files in this directory
  -log_link string
    	If non-empty, add symbolic links in this directory to the log files
  -logbuflevel int
    	Buffer log messages logged at this level or lower (-1 means don't buffer; 0 means buffer INFO only; ...). Has limited applicability on non-prod platforms.
  -logtostderr
    	log to standard error instead of files
  -stack string
    	CloudFormation stack name
  -stderrthreshold value
    	logs at or above this threshold go to stderr (default 2)
  -v value
    	log level for V logs
  -vmodule value
    	comma-separated list of pattern=N settings for file-filtered logging
```

To migrate your existing CDK infrastructure to `pulumi-cdk`:

1. Follow instructions in the [pulumi/pulumi-cdk](https://github.com/pulumi/pulumi-cdk) repo to embed your CDK stacks in a Pulumi program

1. Instead of running `pulumi up`, run `pulumi plugin run cdk-importer -- -stack $CFStackName`. This will import the state of the 
  infrastructure defined by your SDK stack into Pulumi state. This operation is read-only and should not modify any resources.

1. To verify that everything worked as expected, run `pulumi preview`. It should show no changes.
