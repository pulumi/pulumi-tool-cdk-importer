package common

type StackName string         // CloudFormation stack name, ex. t0yv0-cdk-test-app-dev
type ResourceType string      // ex. AWS::S3::Bucket
type PrimaryResourceID string // ex. "${DatabaseName}|${TableName}"
type LogicalResourceID string // ex. t0yv0Bucket1EAC1B2B
type PhysicalResourceID string
