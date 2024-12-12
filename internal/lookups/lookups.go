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
	FindPrimaryResourceID(ctx context.Context, resourceType tokens.Type, logicalID common.LogicalResourceID, props map[string]interface{}) (common.PrimaryResourceID, error)
	FindLogicalResourceID(urn urn.URN) (common.LogicalResourceID, error)
	GetCfnStackResources() map[common.LogicalResourceID]cfnStackResource
	GetRegion() string
	GetAccount() string
}

type cfnStackResource struct {
	ResourceType common.ResourceType
	PhysicalID   common.PhysicalResourceID
	LogicalID    common.LogicalResourceID
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

// getIdentifiers gets the primary identifier from the
// CFN metadata
func getIdentifiers(
	ctx context.Context,
	metadata metadata.MetadataSource,
	resourceToken tokens.Type,
) (common.ResourceType, []resource.PropertyKey, error) {
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

// Correlate and do a best guess to find a CF Logical ID based on a Pulumi URN.
//
// If pulumi-cdk could be instrumented to return this mapping on a side channel this logic would not need to guess.
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
