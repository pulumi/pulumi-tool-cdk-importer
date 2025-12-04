# Why Importing Directly from CloudFormation Templates is Hard

This document outlines the technical challenges involved in building a tool that imports resources directly from a CloudFormation template without executing a corresponding Pulumi program or CDK app.

## The Core Problem: Identity Resolution

To import a resource into Pulumi, we need its **Primary Identifier**.
*   **Simple Resources**: For an S3 bucket, the Primary ID is just the bucket name. This is easy.
*   **Complex Resources**: For an EC2 Route, the Primary ID is a composite of `RouteTableId` + `DestinationCidrBlock`.
*   **Mismatched IDs**: For an IAM Role, the CloudFormation Physical ID is the *Name*, but the CCAPI Primary ID is the *ARN*.

## Challenge 1: Resolving Composite Identifiers

To find the Primary Identifier for complex resources, we typically need to query the AWS Cloud Control API (CCAPI).
*   **The API Call**: `ListResources(Type="AWS::EC2::Route", ResourceModel={RouteTableId: "rtb-123"})`
*   **The Requirement**: To make this call, we need the input properties (like `RouteTableId`) to build the filter model.

## Challenge 2: Parsing CloudFormation Templates

In a static CloudFormation template, these input properties are rarely simple strings. They are almost always references:

```yaml
MyRoute:
  Type: AWS::EC2::Route
  Properties:
    RouteTableId: !Ref MyRouteTable  # Reference to another resource
    GatewayId: !Ref MyInternetGateway
```

To resolve `!Ref MyRouteTable` to the actual ID `rtb-123`, a tool would need to:
1.  **Parse the Template**: Handle JSON/YAML and all intrinsic functions (`Ref`, `Fn::GetAtt`, `Fn::Sub`, `Fn::ImportValue`, `Fn::Select`, etc.).
2.  **Build a Dependency Graph**: Resolve resources in order.
3.  **Simulate Deployment**: Some values (like an IP address allocated during creation) are *only* known after the referenced resource has been deployed. A static parser cannot know these values.

**Conclusion**: Accurately resolving properties from a template requires re-implementing a significant portion of the CloudFormation engine.

## Challenge 3: The "List All" Fallback Limitation

An alternative approach is to ignore the input properties and just list **all** resources of a given type, then filter them client-side using the Physical ID (which we can get from the stack summary).

**Why this fails:**
1.  **Performance**: Listing all `AWS::S3::Object` resources in a bucket is prohibitively slow.
2.  **Mandatory Filters**: Some AWS APIs *require* a filter.
    *   *Example*: `AWS::ElasticLoadBalancingV2::Listener` throws an error if you try to list it without providing a `LoadBalancerArn`.
    *   Since we can't resolve the `LoadBalancerArn` from the template (see Challenge 2), we cannot look up this resource at all.
3.  **Service-Specific Gaps**: Some list operations are scoped or incomplete.
    *   *Example*: `AWS::Events::Rule` Physical ID is `{eventBusName}|{ruleName}` but the primary ID is the ARN. CCAPI `ListResources` only returns rules on the **default** event bus, so non-default buses can never be discovered by listing.
    *   Without a custom resolver, we cannot import rules on other buses even if we know the Physical ID.

**Practical workaround:** we added a `StrategyCustom` path and an Events Rule resolver that, when the physical ID is composite, calls `DescribeRule` to fetch the ARN directly (bypassing CCAPI list). These bespoke resolvers keep the importer working when listing is impossible.

## Summary

Importing from a CloudFormation template is difficult because **resolving resource identity requires resolved input properties**, and **resolving input properties requires a full CloudFormation evaluation engine**.

The current approach (running the CDK/Pulumi program) works because the program execution handles all this complexity for us, passing fully resolved values to the provider.
