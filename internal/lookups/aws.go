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
		return a.findOwnAwsId(resourceType, logicalID, idParts[0])
	default:
		// if there are multiple primary identifiers, then we probably need to use all of them
		// to find the resource. There is also probably a resource model that we need to use
		// in the CCAPI ListResources API call
		resourceModel, err := renderResourceModel(idParts, props, func(s string) string {
			return s
		})
		if err != nil {
			return "", err
		}
		separator := metadataSource.Separator(resourceToken)
		return a.findAwsCompositeId(logicalID, resourceModel, separator)
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

// renderAwsImportId will create a AWS import id value.
// AWS import ids are concatenated with the given separator (usually '/' but some resources use ':')
func renderAwsImportId(id string, resourceModel map[string]string, separator string) string {
	prefix := ""
	for _, value := range resourceModel {
		prefix = fmt.Sprintf("%s%s%s", prefix, value, separator)
	}
	return fmt.Sprintf("%s%s", prefix, id)
}

// findAwsCompositeId returns a lookup id for an AWS resource where
// the lookup id contains multiple values
func (a *awsLookups) findAwsCompositeId(
	logicalID common.LogicalResourceID,
	resourceModel map[string]string,
	separator string,
) (common.PrimaryResourceID, error) {
	if r, ok := a.cfnStackResources[logicalID]; ok {
		suffix := string(r.PhysicalID)
		return common.PrimaryResourceID(renderAwsImportId(suffix, resourceModel, separator)), nil
	}
	return "", fmt.Errorf("Couldn't find an import id for resource with logicalID %q", logicalID)
}

// findOwnAwsId should only be used when the resource only has a single element in it's identifier
func (a *awsLookups) findOwnAwsId(
	resourceType common.ResourceType,
	logicalID common.LogicalResourceID,
	primaryID resource.PropertyKey,
) (common.PrimaryResourceID, error) {
	idPropertyName := strings.ToLower(string(primaryID))
	if strings.HasSuffix(idPropertyName, "name") || strings.HasSuffix(idPropertyName, "id") {
		if r, ok := a.cfnStackResources[logicalID]; ok {
			// NOTE: Assuming that PrimaryResourceID matches the PhysicalID.
			return common.PrimaryResourceID(r.PhysicalID), nil
		}
		return "", fmt.Errorf("Resource doesn't exist in this stack which isn't possible!")
	} else if strings.HasSuffix(idPropertyName, "arn") {
		if r, ok := a.cfnStackResources[logicalID]; ok {
			return a.getArnForResource(resourceType, string(r.PhysicalID))
		}
		return "", fmt.Errorf("Finding resource ids by Arn for resource type %q is not yet supported", resourceType)
	} else {
		return "", fmt.Errorf("Expected suffix of 'Id', 'Name', or 'Arn'; got %s for resource with logicalId %q", idPropertyName, logicalID)
	}
}
