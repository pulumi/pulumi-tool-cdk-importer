// Copyright 2016-2024, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"github.com/pulumi/pulumi-aws-native/provider/pkg/naming"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type StackName string // CloudFormation stack name, ex. t0yv0-cdk-test-app-dev
type AwsClassicBinLocation string
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
	metadata MetadataSource,
) (LogicalResourceID, error) {
	resourceToken := urn.Type()
	resourceType, ok := metadata.ResourceType(resourceToken)
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

func getIdentifiers(
	ctx context.Context,
	metadata MetadataSource,
	resourceToken tokens.Type,
) (ResourceType, []resource.PropertyKey, error) {
	resourceType, ok := metadata.ResourceType(resourceToken)
	if !ok {
		return "", nil, fmt.Errorf("Unknown resource type: %v", resourceToken)
	}
	idParts, ok := metadata.PrimaryIdentifier(resourceToken)
	if !ok {
		return "", nil, fmt.Errorf("Unknown primary ID: %v", resourceToken)
	}
	return resourceType, idParts, nil
}

func (c *ccapi) findClassicPrimaryResourceID(
	ctx context.Context,
	resourceToken tokens.Type,
	logicalID LogicalResourceID,
	props map[string]any,
) (PrimaryResourceID, error) {
	resourceType, idParts, err := getIdentifiers(ctx, awsClassicMetadata, resourceToken)
	if err != nil {
		return "", err
	}
	switch len(idParts) {
	case 0:
		return "", fmt.Errorf("Cannot have 0 ID parts")
	case 1:
		return c.findOwnClassicId(ctx, resourceType, logicalID, idParts[0])
	default:
		resourceModel, err := renderResourceModel(idParts, props, func(s string) string {
			return s
		})
		if err != nil {
			return "", err
		}
		return c.findClassicCompositeId(ctx, resourceType, logicalID, resourceModel)
	}
}

func (c *ccapi) findNativePrimaryResourceID(
	ctx context.Context,
	resourceToken tokens.Type,
	logicalID LogicalResourceID,
	props map[string]any,
) (PrimaryResourceID, error) {
	resourceType, idParts, err := getIdentifiers(ctx, awsNativeMetadata, resourceToken)
	if err != nil {
		return "", err
	}
	switch len(idParts) {
	case 0:
		return "", fmt.Errorf("Cannot have 0 ID parts")
	case 1:
		return c.findOwnNativeId(ctx, resourceType, logicalID, idParts[0])
	default:
		resourceModel, err := renderResourceModel(idParts, props, func(s string) string {
			return naming.ToCfnName(string(s), nil)
		})
		if err != nil {
			return "", err
		}
		return c.findNativeCompositeId(ctx, resourceType, logicalID, resourceModel)
	}
}

func renderResourceModel(idParts []resource.PropertyKey, props map[string]any, resourceKey func(string) string) (map[string]string, error) {
	model := map[string]string{}
	for _, part := range idParts {
		cfnName := resourceKey(string(part))
		if prop, ok := props[cfnName]; ok {
			if val, ok := prop.(string); ok {
				model[cfnName] = val
			} else {
				return nil, fmt.Errorf("id property %s is not a string", prop)
			}
		}
	}
	return model, nil
}

func (c *ccapi) findNativeCompositeId(
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

func (c *ccapi) findClassicCompositeId(
	ctx context.Context,
	resourceType ResourceType,
	logicalID LogicalResourceID,
	resourceModel map[string]string,
) (PrimaryResourceID, error) {
	if r, ok := c.cfnStackResources[logicalID]; ok {
		suffix := string(r.PhysicalID)
		return PrimaryResourceID(renderClassicId(suffix, resourceModel)), nil
	}
	return "", fmt.Errorf("Couldn't find id")
}

func renderClassicId(id string, resourceModel map[string]string) string {
	prefix := ""
	for _, value := range resourceModel {
		prefix = fmt.Sprintf("%s%s/", prefix, value)
	}
	return fmt.Sprintf("%s%s", prefix, id)
}

// findOwnId should only be used when the resource only has a single element in it's identifier
func (c *ccapi) findOwnClassicId(
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
		return "", fmt.Errorf("Finding resource ids by Arn is not yet supported")
	} else {
		return "", fmt.Errorf("Expected suffix of 'Id', 'Name', or 'Arn'; got %s", idPropertyName)
	}
}

// findOwnId should only be used when the resource only has a single element in it's identifier
func (c *ccapi) findOwnNativeId(
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
			// TODO: debug logging of some form
			fmt.Printf("ResourceType %q not yet supported by cloudcontrol, manual mapping required: %s",
				resourceType, err.Error())
			return "<PLACEHOLDER>", nil
		}
		return "", fmt.Errorf("Error finding resource of type %s with resourceModel: %v", resourceType, resourceModel)
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
