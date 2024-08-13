package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type StackName string         // CloudFormation stack name, ex. t0yv0-cdk-test-app-dev
type ResourceType string      // ex. AWS::S3::Bucket
type PrimaryResourceID string // ex. "${DatabaseName}|${TableName}"
type LogicalResourceID string // ex. t0yv0Bucket1EAC1B2B
type PhysicalResourceID string

type cfnStackResource struct {
	ResourceType ResourceType
	PhysicalID   PhysicalResourceID
	LogicalID    LogicalResourceID
}

func newCcapi(ctx context.Context) (*ccapi, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	client := cloudcontrol.NewFromConfig(cfg)
	cfnClient := cloudformation.NewFromConfig(cfg)
	return &ccapi{
		ccapiClient:        client,
		cfnClient:          cfnClient,
		cfnStackResources:  make(map[LogicalResourceID]cfnStackResource),
		ccapiResourceCache: make(map[ResourceType][]types.ResourceDescription),
	}, nil
}

type ccapi struct {
	ccapiClient        *cloudcontrol.Client
	cfnClient          *cloudformation.Client
	cfnStackResources  map[LogicalResourceID]cfnStackResource
	ccapiResourceCache map[ResourceType][]types.ResourceDescription
}

// Correlate and do a best guess to find a CF Logical ID based on a Pulumi URN.
//
// If pulumi-cdk could be instrumented to return this mapping on a side channel this logic would not need to guess.
func (c *ccapi) findLogicalResourceID(
	ctx context.Context,
	urn resource.URN,
) (LogicalResourceID, error) {
	resourceToken := urn.Type()
	resourceType, ok := awsNativeMetadata.ResourceType(resourceToken)
	if !ok {
		return "", fmt.Errorf("Unknown resource type: %v", resourceToken)
	}
	matchCount := 0
	var match cfnStackResource
	for _, r := range c.cfnStackResources {
		if r.ResourceType != resourceType {
			continue
		}
		if strings.Contains(strings.ToLower(string(r.LogicalID)), strings.ToLower(urn.Name())) {
			match = r
			matchCount++
		}
	}
	if matchCount == 0 {
		return "", fmt.Errorf("No matching CF resources for URN %v", urn)
	}
	if matchCount > 1 {
		return "", fmt.Errorf("Conflicting matching CF resources for URN %v", urn)
	}
	return match.LogicalID, nil
}

func (c *ccapi) findPrimaryResourceID(
	ctx context.Context,
	resourceToken tokens.Type,
	logicalID LogicalResourceID,
) (PrimaryResourceID, error) {
	resourceType, ok := awsNativeMetadata.ResourceType(resourceToken)
	if !ok {
		return "", fmt.Errorf("Unknown resource type: %v", resourceToken)
	}
	idParts, ok := awsNativeMetadata.PrimaryIdentifier(resourceToken)
	if !ok {
		return "", fmt.Errorf("Unknown primary ID: %v", resourceToken)
	}
	switch len(idParts) {
	case 0:
		return "", fmt.Errorf("Cannot have 0 ID parts")
	case 1:
		return c.findOwnId(ctx, resourceType, logicalID, idParts[0])
	default:
		return c.findCompositeId(ctx, resourceType, logicalID, nil)
	}
}

func (c *ccapi) findCompositeId(
	ctx context.Context,
	resourceType ResourceType,
	logicalID LogicalResourceID,
	resourceModel map[string]string,
) (PrimaryResourceID, error) {
	if r, ok := c.cfnStackResources[logicalID]; ok {
		suffix := string(r.PhysicalID)
		id, err := c.findResourceIdentifierBySuffix(ctx, resourceType, suffix, resourceModel)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return "", fmt.Errorf("Couldn't find id")
}

// findOwnId should only be used when the resource only has a single element in it's identifier
func (c *ccapi) findOwnId(
	ctx context.Context,
	resourceType ResourceType,
	logicalID LogicalResourceID,
	primaryID resource.PropertyKey,
) (PrimaryResourceID, error) {
	idPropertyName := strings.ToLower(string(primaryID))
	if strings.HasSuffix(idPropertyName, "name") || strings.HasSuffix(idPropertyName, "id") {
		if r, ok := c.cfnStackResources[logicalID]; ok {
			// NOTE! Assuming that PrimaryResourceID matches the PhysicalID.
			return PrimaryResourceID(r.PhysicalID), nil
		}
		return "", fmt.Errorf("Resource doesn't exist in this stack which isn't possible!")
	} else if strings.HasSuffix(idPropertyName, "arn") {
		if r, ok := c.cfnStackResources[logicalID]; ok {
			suffix := string(r.PhysicalID)
			id, err := c.findResourceIdentifierBySuffix(ctx, resourceType, suffix, nil)
			if err != nil {
				return "", fmt.Errorf("Could not find id for %s: %w", logicalID, err)
			}
			return id, nil
		}
	} else {
		return "", fmt.Errorf("Expected suffix of 'Id', 'Name', or 'Arn'; got %s", idPropertyName)
	}
	return "", fmt.Errorf("Something happened")
}

func (c *ccapi) listResources(
	ctx context.Context,
	resourceType ResourceType,
	resourceModel map[string]string,
) ([]types.ResourceDescription, error) {
	if val, ok := c.ccapiResourceCache[resourceType]; ok {
		return val, nil
	}

	var model *string
	if len(resourceModel) > 0 {
		val, err := json.Marshal(resourceModel)
		if err != nil {
			return nil, err
		}
		stringVal := string(val)
		model = &stringVal
	}

	typeName := string(resourceType)
	paginator := cloudcontrol.NewListResourcesPaginator(c.ccapiClient, &cloudcontrol.ListResourcesInput{
		ResourceModel: model,
		TypeName:      &typeName,
	})
	resources := []types.ResourceDescription{}
	// TODO: we might be able to short circuit this if we find the correct one
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		resources = append(resources, output.ResourceDescriptions...)
	}

	c.ccapiResourceCache[resourceType] = resources
	return resources, nil
}

// This finds resources with Arn identifiers based on whether the Arn ends
// in the provided value
func (c *ccapi) findResourceIdentifierBySuffix(
	ctx context.Context,
	resourceType ResourceType,
	suffix string,
	resourceModel map[string]string,
) (PrimaryResourceID, error) {
	resources, err := c.listResources(ctx, resourceType, resourceModel)
	if err != nil {
		var uae *types.UnsupportedActionException
		if errors.As(err, &uae) {
			// TODO debug logging of some form
			fmt.Printf("ResourceType %q not yet supported by cloudcontrol, manual mapping required",
				resourceType)
		}
		return "<PLACEHOLDER>", nil
	}

	for _, resource := range resources {
		if resource.Identifier != nil && strings.HasSuffix(*resource.Identifier, suffix) {
			return PrimaryResourceID(*resource.Identifier), nil
		}
	}

	return "", fmt.Errorf("could not find resource identifier for type: %s", resourceType)
}

func (c *ccapi) getStackResources(ctx context.Context, stackName StackName) error {
	sn := string(stackName)
	paginator := cloudformation.NewListStackResourcesPaginator(c.cfnClient, &cloudformation.ListStackResourcesInput{
		StackName: &sn,
	})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, s := range output.StackResourceSummaries {
			if s.PhysicalResourceId == nil || s.LogicalResourceId == nil || s.ResourceType == nil {
				continue
			}
			r := cfnStackResource{
				ResourceType: ResourceType(*s.ResourceType),
				LogicalID:    LogicalResourceID(*s.LogicalResourceId),
				PhysicalID:   PhysicalResourceID(*s.PhysicalResourceId),
			}
			c.cfnStackResources[r.LogicalID] = r
		}
	}
	return nil
}
