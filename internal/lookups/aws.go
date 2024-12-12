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
	cfnStackResources map[common.LogicalResourceID]cfnStackResource
}

func NewAwsLookups(
	resources map[common.LogicalResourceID]cfnStackResource,
	region string,
	account string,
) *awsLookups {
	return &awsLookups{
		region:            region,
		account:           account,
		cfnStackResources: resources,
	}
}

func (a *awsLookups) GetCfnStackResources() map[common.LogicalResourceID]cfnStackResource {
	return a.cfnStackResources
}

func (a *awsLookups) GetRegion() string {
	return a.region
}

func (a *awsLookups) GetAccount() string {
	return a.account
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
	resourceType, idParts, err := getIdentifiers(ctx, metadata.NewAwsMetadataSource(), resourceToken)
	if err != nil {
		return "", err
	}
	switch len(idParts) {
	case 0:
		return "", fmt.Errorf("Cannot have 0 ID parts")
	case 1:
		return a.findOwnClassicId(ctx, resourceType, logicalID, idParts[0])
	default:
		resourceModel, err := renderResourceModel(idParts, props, func(s string) string {
			return s
		})
		if err != nil {
			return "", err
		}
		return a.findClassicCompositeId(logicalID, resourceModel)
	}
}

func (a *awsLookups) getArnForResource(resourceType common.ResourceType, name string) (common.PrimaryResourceID, error) {
	switch resourceType {
	case "AWS::IAM::Policy":
		return common.PrimaryResourceID(fmt.Sprintf("arn:aws:iam::%s:policy/%s", a.account, name)), nil
	}
	return "", fmt.Errorf("Arn lookup for resourceType %q not supported", resourceType)
}

func renderClassicId(id string, resourceModel map[string]string) string {
	prefix := ""
	for _, value := range resourceModel {
		prefix = fmt.Sprintf("%s%s/", prefix, value)
	}
	return fmt.Sprintf("%s%s", prefix, id)
}

func (a *awsLookups) findClassicCompositeId(
	logicalID common.LogicalResourceID,
	resourceModel map[string]string,
) (common.PrimaryResourceID, error) {
	if r, ok := a.cfnStackResources[logicalID]; ok {
		suffix := string(r.PhysicalID)
		return common.PrimaryResourceID(renderClassicId(suffix, resourceModel)), nil
	}
	return "", fmt.Errorf("Couldn't find id")
}

// findOwnId should only be used when the resource only has a single element in it's identifier
func (a *awsLookups) findOwnClassicId(
	ctx context.Context,
	resourceType common.ResourceType,
	logicalID common.LogicalResourceID,
	primaryID resource.PropertyKey,
) (common.PrimaryResourceID, error) {
	idPropertyName := strings.ToLower(string(primaryID))
	if strings.HasSuffix(idPropertyName, "name") || strings.HasSuffix(idPropertyName, "id") {
		if r, ok := a.cfnStackResources[logicalID]; ok {
			// NOTE! Assuming that PrimaryResourceID matches the PhysicalID.
			return common.PrimaryResourceID(r.PhysicalID), nil
		}
		return "", fmt.Errorf("Resource doesn't exist in this stack which isn't possible!")
	} else if strings.HasSuffix(idPropertyName, "arn") {
		if r, ok := a.cfnStackResources[logicalID]; ok {
			return a.getArnForResource(resourceType, string(r.PhysicalID))
		}
		return "", fmt.Errorf("Finding resource ids by Arn is not yet supported")
	} else {
		return "", fmt.Errorf("Expected suffix of 'Id', 'Name', or 'Arn'; got %s", idPropertyName)
	}
}
