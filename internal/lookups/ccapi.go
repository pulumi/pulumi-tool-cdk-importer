package lookups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/pulumi/pulumi-aws-native/provider/pkg/naming"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// ListResourcesPager is an interface for cloudcontrol.ListResourcesPaginator
type ListResourcesPager interface {
	HasMorePages() bool
	NextPage(ctx context.Context, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error)
}

// A key to use for caching resources
type resourceCacheKey string

// makeCacheKey creates a cache key for a resource based on its type and model
// This combination should create a unique key for each resource
func makeCacheKey(resourceType common.ResourceType, resourceModel map[string]string) resourceCacheKey {
	key := string(resourceType)
	for k, v := range resourceModel {
		key += k + v
	}
	return resourceCacheKey(key)
}

// CCAPIClient is a client for the cloudcontrol API
type CCAPIClient interface {
	GetPager(typeName string, resourceModel *string) ListResourcesPager
}

type ccapiClient struct {
	client *cloudcontrol.Client
}

// GetPager gets a ListResourcesPager for a given type and resource model
func (c *ccapiClient) GetPager(typeName string, resourceModel *string) ListResourcesPager {
	return cloudcontrol.NewListResourcesPaginator(c.client, &cloudcontrol.ListResourcesInput{
		TypeName:      &typeName,
		ResourceModel: resourceModel,
	})
}

type ccapiLookups struct {
	ccapiClient        CCAPIClient
	cfnStackResources  map[common.LogicalResourceID]CfnStackResource
	ccapiResourceCache map[resourceCacheKey][]types.ResourceDescription
}

func NewCCApiLookups(ctx context.Context, client *cloudcontrol.Client, cfnStackResources map[common.LogicalResourceID]CfnStackResource) (*ccapiLookups, error) {
	return &ccapiLookups{
		ccapiClient:        &ccapiClient{client: client},
		cfnStackResources:  cfnStackResources,
		ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
	}, nil
}

func (c *ccapiLookups) FindLogicalResourceID(
	urn resource.URN,
) (common.LogicalResourceID, error) {
	return findLogicalResourceID(urn, metadata.NewCCApiMetadataSource(), c.cfnStackResources)
}

// First find the primary identifier of the resource in the CFN schema
func (c *ccapiLookups) FindPrimaryResourceID(
	ctx context.Context,
	resourceToken tokens.Type,
	logicalID common.LogicalResourceID,
	props map[string]any,
) (common.PrimaryResourceID, error) {
	c.cfnStackResources[logicalID] = CfnStackResource{
		ResourceType: c.cfnStackResources[logicalID].ResourceType,
		LogicalID:    logicalID,
		PhysicalID:   c.cfnStackResources[logicalID].PhysicalID,
		Props:        props,
	}
	resourceType, idParts, err := getPrimaryIdentifiers(metadata.NewCCApiMetadataSource(), resourceToken)
	if err != nil {
		return "", err
	}
	switch len(idParts) {
	case 0:
		return "", fmt.Errorf("ResourceType %q with logicalID %q has no primary identifiers", resourceType, logicalID)
	case 1:
		return c.findOwnNativeId(ctx, resourceType, logicalID, idParts[0])
	default:
		resourceModel, err := renderResourceModel(idParts, props, func(s string) string {
			return naming.ToCfnName(string(s), nil)
		})
		if err != nil {
			return "", err
		}
		return c.findCCApiCompositeId(ctx, resourceType, logicalID, resourceModel)
	}
}

// findCCApiCompositeId attempts to find the resource where the identifier is a composite id made up
// of multiple parts
func (c *ccapiLookups) findCCApiCompositeId(
	ctx context.Context,
	resourceType common.ResourceType,
	logicalID common.LogicalResourceID,
	resourceModel map[string]string,
) (common.PrimaryResourceID, error) {
	if r, ok := c.cfnStackResources[logicalID]; ok {
		suffix := string(r.PhysicalID)
		id, err := c.findResourceIdentifier(ctx, resourceType, logicalID, suffix, resourceModel)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return "", fmt.Errorf("Couldn't find id")
}

// findOwnId should only be used when the resource only has a single element in it's identifier
func (c *ccapiLookups) findOwnNativeId(
	ctx context.Context,
	resourceType common.ResourceType,
	logicalID common.LogicalResourceID,
	primaryID resource.PropertyKey,
) (common.PrimaryResourceID, error) {
	idPropertyName := strings.ToLower(string(primaryID))
	if strings.HasSuffix(idPropertyName, "name") || strings.HasSuffix(idPropertyName, "id") {
		if r, ok := c.cfnStackResources[logicalID]; ok {
			// NOTE! Assuming that PrimaryResourceID matches the PhysicalID.
			return common.PrimaryResourceID(r.PhysicalID), nil
		}
		return "", fmt.Errorf("Resource doesn't exist in this stack which isn't possible!")
	} else if strings.HasSuffix(idPropertyName, "arn") {
		if r, ok := c.cfnStackResources[logicalID]; ok {
			suffix := string(r.PhysicalID)
			id, err := c.findResourceIdentifier(ctx, resourceType, logicalID, suffix, nil)
			if err != nil {
				return "", fmt.Errorf("Could not find id for %s: %w", logicalID, err)
			}
			return id, nil
		}
	} else if resourceType == "AWS::S3::BucketPolicy" && idPropertyName == "bucket" {
		if r, ok := c.cfnStackResources[logicalID]; ok {
			// NOTE! Assuming that PrimaryResourceID matches the PhysicalID.
			return common.PrimaryResourceID(r.PhysicalID), nil
		}
		return "", fmt.Errorf("Resource doesn't exist in this stack which isn't possible!")
	} else {
		return "", fmt.Errorf("Expected suffix of 'Id', 'Name', or 'Arn'; got %s", idPropertyName)
	}
	return "", fmt.Errorf("Something happened")
}

// findResourceIdentifier attempts to determine an import id for a resource when
// it is not a simple case of using the PhysicalID.
// It will first list all resources of the given type from the CCAPI and then try to find the
// specific resource based on whether it starts with or ends with the suffix. Unfortunately
// CCAPI is not consistent with how a composite resource id is constructed, sometimes the PhysicalID is
// at the start and sometimes at the end. e.g. `apiId|stageName` or `stageName|apiId`
func (c *ccapiLookups) findResourceIdentifier(
	ctx context.Context,
	resourceType common.ResourceType,
	logicalID common.LogicalResourceID,
	suffix string,
	resourceModel map[string]string,
) (common.PrimaryResourceID, error) {
	resources, err := c.listResources(ctx, resourceType, resourceModel)
	if err != nil {
		var uae *types.UnsupportedActionException
		var invalid *types.InvalidRequestException
		if errors.As(err, &uae) {
			// TODO: debug logging of some form
			fmt.Printf("ResourceType %q not yet supported by cloudcontrol, manual mapping required: %s",
				resourceType, err.Error())
			return "<PLACEHOLDER>", nil
		} else if errors.As(err, &invalid) {
			// Then we missed something in the resource model. Try to extract what that might be
			// The schema does not always contain all the required information to determine what the
			// CCAPI ListResources resource model should be. For example, AWS::ElasticLoadBalancingV2::Listener
			// I've found that the logs usually look like this (hopefully this is consistent):
			// `InvalidRequestException: Missing Or Invalid ResourceModel property in AWS::ElasticLoadBalancingV2::Listener list handler request input. Required property: [LoadBalancerArn]`
			re := regexp.MustCompile(`Required property: \[(.*?)\]`)
			match := re.FindStringSubmatch(invalid.ErrorMessage())
			if len(match) > 1 {
				missingProperty := match[1]
				// create a correct resource model with the missing property
				resourceModel, err = renderResourceModel([]resource.PropertyKey{
					resource.PropertyKey(missingProperty),
				}, c.cfnStackResources[logicalID].Props, func(s string) string {
					return s
				})
				if err != nil {
					return "", fmt.Errorf("Error rendering resource model: %w", err)
				}
				// run it again with the new resource model
				return c.findResourceIdentifier(ctx, resourceType, logicalID, suffix, resourceModel)
			} else {
				return "", fmt.Errorf("Error finding resource of type %s with resourceModel: %v: %w", resourceType, resourceModel, err)
			}
		} else {
			return "", fmt.Errorf("Error finding resource of type %s with resourceModel: %v: %w", resourceType, resourceModel, err)
		}
	}

	for _, resource := range resources {
		if resource.Identifier != nil && (strings.HasSuffix(*resource.Identifier, suffix) ||
			strings.HasPrefix(*resource.Identifier, suffix)) {
			return common.PrimaryResourceID(*resource.Identifier), nil
		} else {
			// TODO: debug logging
		}
	}

	return "", fmt.Errorf("could not find resource identifier for type: %s", resourceType)
}

// listResources lists resources of a given type from the CCAPI
func (c *ccapiLookups) listResources(
	ctx context.Context,
	resourceType common.ResourceType,
	resourceModel map[string]string,
) ([]types.ResourceDescription, error) {
	cacheKey := makeCacheKey(resourceType, resourceModel)
	if val, ok := c.ccapiResourceCache[cacheKey]; ok {
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
	paginator := c.ccapiClient.GetPager(typeName, model)
	resources := []types.ResourceDescription{}
	// TODO: we might be able to short circuit this if we find the correct one
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		resources = append(resources, output.ResourceDescriptions...)
	}

	c.ccapiResourceCache[cacheKey] = resources
	return resources, nil
}
