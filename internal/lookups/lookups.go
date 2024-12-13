package lookups

import (
	"context"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type Lookups interface {
	// FindPrimarResourceID attempts to find the primary identifier for a resource.
	// This id is what will be used to run the import of the resource
	FindPrimaryResourceID(ctx context.Context, resourceType tokens.Type, logicalID common.LogicalResourceID, props map[string]interface{}) (common.PrimaryResourceID, error)

	// FindLogicalResourceID attempts to find the logical resource id for a given URN.
	// The logical resource id is the id that is used to correlate the Pulumi resource with the CFN resource
	FindLogicalResourceID(urn urn.URN) (common.LogicalResourceID, error)

	// GetCfnStackResources returns the Resources in the CloudFormation stack
	GetCfnStackResources() map[common.LogicalResourceID]cfnStackResource

	// GetRegion returns the region of the stack
	GetRegion() string

	// GetAccount returns the account of the stack
	GetAccount() string
}

// cfnStackResource represents a CloudFormation resource
type cfnStackResource struct {
	// The type of the resource, i.e. AWS::S3::Bucket
	ResourceType common.ResourceType

	// The CloudFormation Physical ID of the resource
	// See https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/resources-section-structure.html#resources-section-physical-id
	PhysicalID common.PhysicalResourceID

	// The CloudFormation Logical ID of the resource
	// See https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/resources-section-structure.html#resources-section-logical-id
	LogicalID common.LogicalResourceID

	// The Input properties for this resource
	Props map[string]any
}

// renderResourceModel create a CCAPI resource model to use when making a
// CCAPI ListResources API call.
// There is no schema for the resource model so we have to use some heuristics
//
// idParts are the properties that make up the primary identifier of the resource, i.e. [apiId, routeId]
// props are the input properties of the resource
// resourceKey is a function that can be used to transform a value, e.g. convert to a CFN name
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

// getPrimaryIdentifiers gets the primary identifier from the CFN metadata
func getPrimaryIdentifiers(metadata metadata.MetadataSource, resourceToken tokens.Type) (common.ResourceType, []resource.PropertyKey, error) {
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

// Correlate and do a best guess to find a CFN Logical ID based on a Pulumi URN.
func findLogicalResourceID(
	urn resource.URN,
	metadata metadata.MetadataSource,
	cfnStackResources map[common.LogicalResourceID]cfnStackResource,
) (common.LogicalResourceID, error) {
	resourceToken := urn.Type()
	resourceType, ok := metadata.ResourceType(resourceToken)
	if !ok {
		return "", fmt.Errorf("Unknown resource type: %v", resourceToken)
	}
	matchCount := 0
	var match cfnStackResource
	for _, r := range cfnStackResources {
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
