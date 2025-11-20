# Resource ID Resolution

This document explains how the CDK importer resolves resource identifiers when importing CloudFormation resources into Pulumi.

## Background

When importing a CloudFormation resource into Pulumi, we need to determine the **Primary Resource Identifier** that Pulumi will use to import the resource. This is complicated by the fact that different AWS services use different identifier formats.

### Key Concepts

#### CloudFormation Physical ID
The unique identifier CloudFormation assigns to a resource after creation. This is what you see in the CloudFormation console.

**Examples:**
- S3 Bucket: `my-bucket-name`
- Lambda Function: `my-function-name`
- IAM Role: `MyRoleName` (just the name, not the ARN)

#### CCAPI Primary Identifier
The identifier(s) required by AWS Cloud Control API to uniquely identify a resource. Defined in the resource's CloudFormation schema.

**Examples:**
- S3 Bucket: `BucketName` (single property)
- EC2 Route: `RouteTableId` + `DestinationCidrBlock` (composite - multiple properties)
- IAM Role: `Arn` (single property, but different from Physical ID)

#### The Problem
The Physical ID doesn't always match the Primary Identifier:
- Sometimes they're the same (S3 Bucket)
- Sometimes the Primary Identifier is an ARN but Physical ID is a name (IAM Role)
- Sometimes the Primary Identifier is composite (EC2 Route)
- Sometimes the schema is incomplete and we need to discover required properties at runtime

## Resolution Strategy

### Single-Property Identifiers

When a resource has a single primary identifier property, we use a three-tier approach:

#### 1. Explicit Strategy Override
Check `internal/metadata/ccapi.go` for an explicit `IdPropertyStrategy`:
- `StrategyPhysicalID`: Use the CloudFormation Physical ID directly
- `StrategyLookup`: Perform a CCAPI lookup to find the identifier

```go
idPropertyStrategies: map[string]map[string]IdPropertyStrategy{
    // Only add entries for resources where default behavior doesn't work
}
```

#### 2. ARN Heuristic
If the property name ends in `arn`, perform a CCAPI lookup:
1. List all resources of that type via CCAPI
2. Find the one whose identifier contains the Physical ID as a prefix or suffix
3. Return the full identifier

**Why?** ARN properties often don't match the Physical ID. For example:
- Physical ID: `MyRole`
- Primary Identifier (Arn): `arn:aws:iam::123456789012:role/MyRole`

#### 3. Default: Physical ID
For all other properties, assume the Physical ID matches the Primary Identifier.

**This works for most resources:**
- Properties ending in `Name`: `BucketName`, `FunctionName`, `TableName`
- Properties ending in `Id`: `VpcId`, `SubnetId`, `SecurityGroupId`
- Other properties: `Cluster`, `Bucket`, etc.

### Composite Identifiers

When a resource has multiple primary identifier properties, we:

1. **Render a resource model** from the available properties
2. **Call CCAPI ListResources** with that model
3. **Handle missing properties** via retry logic (see below)
4. **Match by Physical ID** - find the resource whose identifier contains the Physical ID

**Example: `AWS::ApplicationAutoScaling::ScalingPolicy`**
- Primary Identifiers: `Arn` + `ScalableDimension`
- But `Arn` is not in the input props (it's computed)
- So we render model with just `ScalableDimension`
- CCAPI returns error: "Missing required property: ServiceNamespace"
- We extract `ServiceNamespace` from error and retry
- CCAPI returns the full identifier: `arn:aws:...|ecs:service:DesiredCount`

### Missing Property Retry Logic

Some resources require properties in the CCAPI `ListResources` call that aren't in the primary identifier schema. We handle this automatically:

1. **Initial call fails** with `InvalidRequestException`
2. **Extract missing property** from error message using regex:
   - `Required property: [PropertyName]`
   - `required key [PropertyName]`
3. **Render new resource model** with just the missing property
4. **Retry the call** with the new model
5. **Return the identifier** from the successful response

In rare cases the missing property isn't in the inputs at all. We attempt to derive it from other fields when we can:
- `AWS::ApplicationAutoScaling::ScalingPolicy`: if `ServiceNamespace` is missing, we derive it from `ScalableDimension` (e.g., `ecs:service:DesiredCount` → `ecs`) or, failing that, the trailing segment of `ScalingTargetId`.

**Example Error Messages:**
```
Missing Or Invalid ResourceModel property in AWS::ElasticLoadBalancingV2::Listener 
list handler request input. Required property: [LoadBalancerArn]

Missing or invalid ResourceModel property in AWS::Lambda::Permission list handler 
request input.Required property:  (#: required key [FunctionName] not found)
```

## Edge Cases

### 1. Primary Identifier Override

Some resources have incorrect primary identifiers in the upstream metadata. We override these in `internal/metadata/ccapi.go`:

```go
primaryIdentifierOverrides: map[string][]string{
    "aws-native:lambda:Permission": {"functionArn", "id"},
}
```

**Why?** The upstream schema says the primary identifier is just `Id`, but CCAPI actually requires `FunctionArn` + `Id`.

### 2. ID Property Strategy Override

For resources where the default behavior doesn't work, we can add an explicit strategy:

```go
idPropertyStrategies: map[string]map[string]IdPropertyStrategy{
    "AWS::SomeService::Resource": {
        "propertyname": StrategyLookup,
    },
}
```

**When to use:**
- Physical ID doesn't match the primary identifier
- Default heuristics don't work
- Need to force a specific behavior

## Code Locations

### Metadata Layer
- `internal/metadata/ccapi.go`: Strategy definitions and overrides
- `internal/metadata/schemas/pulumi-aws-native-metadata.json`: Upstream schema data

### Lookup Logic
- `internal/lookups/ccapi.go`: CCAPI-based lookup implementation
  - `FindPrimaryResourceID()`: Entry point
  - `findOwnNativeId()`: Single-property identifier resolution
  - `findCCApiCompositeId()`: Composite identifier resolution
  - `findResourceIdentifier()`: CCAPI lookup with retry logic
- `internal/lookups/aws.go`: Classic AWS provider lookup (for comparison)

### Tests
- `internal/lookups/ccapi_test.go`: Test cases covering various scenarios
  - Simple identifiers
  - Composite identifiers
  - Missing property retry
  - Strategy overrides

## Adding Support for New Resources

If you encounter a resource that doesn't import correctly:

1. **Check the error message** - does it indicate a missing property?
2. **Check the Physical ID** - does it match what you'd expect for the primary identifier?
3. **Check the schema** - what are the primary identifiers in the metadata?

### If Physical ID doesn't match:
Add an `IdPropertyStrategy` override in `internal/metadata/ccapi.go`:

```go
idPropertyStrategies: map[string]map[string]IdPropertyStrategy{
    "AWS::Service::Resource": {
        "propertyname": StrategyLookup, // or StrategyPhysicalID
    },
}
```

### If primary identifiers are wrong:
Add a `primaryIdentifierOverrides` entry in `internal/metadata/ccapi.go`:

```go
primaryIdentifierOverrides: map[string][]string{
    "aws-native:service:Resource": {"correctProperty1", "correctProperty2"},
}
```

### If CCAPI requires extra properties:
The retry logic should handle this automatically. If it doesn't, check the error message format and update the regex patterns in `findResourceIdentifier()`.

## Design Principles

1. **Default to simple** - Assume Physical ID works unless proven otherwise
2. **Use heuristics** - ARN suffix is a reliable indicator
3. **Fail gracefully** - Clear error messages when lookup fails
4. **Minimize overrides** - Only add explicit overrides for true edge cases
5. **Leverage retry** - Let CCAPI tell us what's missing rather than guessing
