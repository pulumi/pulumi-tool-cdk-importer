package common

// CloudFormation stack name, ex. t0yv0-cdk-test-app-dev
type StackName string

// The CN type of the resource, ex. AWS::S3::Bucket
type ResourceType string

// The ID value that is used to import the resource
type PrimaryResourceID string // ex. "${DatabaseName}|${TableName}"

// The CloudFormation Logical ID of the resource
// See https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/resources-section-structure.html#resources-section-logical-id
type LogicalResourceID string // ex. t0yv0Bucket1EAC1B2B

// The CloudFormation Physical ID of the resource
// See https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/resources-section-structure.html#resources-section-physical-id
type PhysicalResourceID string
