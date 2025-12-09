package lookups

import (
	"context"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type awsLookups struct {
	region            string
	account           string
	cfnStackResources map[common.LogicalResourceID]CfnStackResource
}

func NewAwsLookups(
	resources map[common.LogicalResourceID]CfnStackResource,
	region string,
	account string,
) *awsLookups {
	return &awsLookups{
		region:            region,
		account:           account,
		cfnStackResources: resources,
	}
}

func (c *awsLookups) FindLogicalResourceID(
	urn resource.URN,
) (common.LogicalResourceID, error) {
	return findLogicalResourceID(urn, metadata.NewAwsMetadataSource(), c.cfnStackResources)
}

func (a *awsLookups) FindPrimaryResourceID(
	ctx context.Context,
	resourceToken tokens.Type,
	logicalID common.LogicalResourceID,
	props map[string]any,
) (common.PrimaryResourceID, error) {
	metadataSource := metadata.NewAwsMetadataSource()
	resourceType, idParts, err := getPrimaryIdentifiers(metadataSource, resourceToken)
	if err != nil {
		return "", err
	}
	switch len(idParts) {
	case 0:
		return "", fmt.Errorf("ResourceType %q with logicalID %q has no primary identifiers", resourceType, logicalID)
	case 1:
		// if there is only one primary identifier, then we should be able to
		// use that to find the resource
		return a.findOwnAwsId(resourceType, logicalID, idParts[0], props)
	default:
		// if there are multiple primary identifiers, then we probably need to use all of them
		parts, err := buildIdentifierParts(idParts, props, string(a.cfnStackResources[logicalID].PhysicalID))
		if err != nil {
			return "", err
		}
		separator := metadataSource.Separator(resourceToken)
		return common.PrimaryResourceID(strings.Join(parts, separator)), nil
	}
}

// getArnForResource will create an ARN for the given resourceType and name
// This is needed for some special resources where the id is the arn. In these
// cases the arn will probably not be part of the data that we have so we need to construct it
func (a *awsLookups) getArnForResource(resourceType common.ResourceType, name string) (common.PrimaryResourceID, error) {
	switch resourceType {
	case "AWS::IAM::Policy":
		return common.PrimaryResourceID(fmt.Sprintf("arn:aws:iam::%s:policy/%s", a.account, name)), nil
	}
	return "", fmt.Errorf("Arn lookup for resourceType %q not supported", resourceType)
}

// buildIdentifierParts assembles identifier segments in order using provided props, falling back to
// the CloudFormation physical ID for the first missing segment only. It errors if a segment is
// missing and the physical ID has already been consumed, or if a prop is present but not a string.
func buildIdentifierParts(
	idParts []resource.PropertyKey,
	props map[string]any,
	physicalID string,
) ([]string, error) {
	parts := make([]string, 0, len(idParts))
	usedPhysical := false

	for _, idPart := range idParts {
		if val, ok := props[string(idPart)]; ok {
			if s, ok := val.(string); ok && s != "" {
				parts = append(parts, s)
				continue
			}
			return nil, fmt.Errorf("expected id property %q to be a string; got %v", idPart, val)
		}
		if !usedPhysical && physicalID != "" {
			parts = append(parts, physicalID)
			usedPhysical = true
			continue
		}
		return nil, fmt.Errorf("Couldn't find an import id for identifier part %q", idPart)
	}

	return parts, nil
}

// findOwnAwsId should only be used when the resource only has a single element in it's identifier
func (a *awsLookups) findOwnAwsId(
	resourceType common.ResourceType,
	logicalID common.LogicalResourceID,
	primaryID resource.PropertyKey,
	props map[string]any,
) (common.PrimaryResourceID, error) {
	idPropertyName := strings.ToLower(string(primaryID))

	// Prefer the explicit property value when provided (common for queueUrl-style identifiers).
	if val, ok := props[string(primaryID)]; ok {
		if s, ok := val.(string); ok && s != "" {
			return common.PrimaryResourceID(s), nil
		}
		return "", fmt.Errorf("expected id property %q to be a string; got %v", primaryID, val)
	}

	// If the identifier is an ARN, construct or look it up if we know how.
	if strings.HasSuffix(idPropertyName, "arn") {
		if r, ok := a.cfnStackResources[logicalID]; ok {
			return a.getArnForResource(resourceType, string(r.PhysicalID))
		}
	}

	// Default: assume the PhysicalID is the import identifier, regardless of naming.
	if r, ok := a.cfnStackResources[logicalID]; ok {
		return common.PrimaryResourceID(r.PhysicalID), nil
	}
	return "", fmt.Errorf("Resource doesn't exist in this stack which isn't possible!")
}
