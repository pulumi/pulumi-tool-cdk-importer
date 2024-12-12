package lookups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pulumi/pulumi-aws-native/provider/pkg/naming"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type ccapiLookups struct {
	ccapiClient        *cloudcontrol.Client
	region             string
	account            string
	cfnClient          *cloudformation.Client
	cfnStackResources  map[common.LogicalResourceID]cfnStackResource
	ccapiResourceCache map[common.ResourceType][]types.ResourceDescription
}

func NewCCApiLookups(ctx context.Context) (*ccapiLookups, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	client := cloudcontrol.NewFromConfig(cfg)
	cfnClient := cloudformation.NewFromConfig(cfg)
	stsClient := sts.NewFromConfig(cfg)
	res, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, err
	}
	return &ccapiLookups{
		region:             cfg.Region,
		account:            *res.Account,
		ccapiClient:        client,
		cfnClient:          cfnClient,
		cfnStackResources:  make(map[common.LogicalResourceID]cfnStackResource),
		ccapiResourceCache: make(map[common.ResourceType][]types.ResourceDescription),
	}, nil
}

func (c *ccapiLookups) GetRegion() string {
	return c.region
}

func (c *ccapiLookups) GetAccount() string {
	return c.account
}

func (c *ccapiLookups) GetCfnStackResources() map[common.LogicalResourceID]cfnStackResource {
	return c.cfnStackResources
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
	resourceType, idParts, err := getIdentifiers(ctx, metadata.NewCCApiMetadataSource(), resourceToken)
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

func (c *ccapiLookups) findNativeCompositeId(
	ctx context.Context,
	resourceType common.ResourceType,
	logicalID common.LogicalResourceID,
	resourceModel map[string]string,
) (common.PrimaryResourceID, error) {
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

// This finds resources with Arn identifiers based on whether the Arn ends
// in the provided value
func (c *ccapiLookups) findResourceIdentifierBySuffix(
	ctx context.Context,
	resourceType common.ResourceType,
	suffix string,
	resourceModel map[string]string,
) (common.PrimaryResourceID, error) {
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
		if resource.Identifier != nil && (strings.HasSuffix(*resource.Identifier, suffix) ||
			strings.HasPrefix(*resource.Identifier, suffix)) {
			return common.PrimaryResourceID(*resource.Identifier), nil
		}
	}

	log.New(os.Stderr, "", 0).Println("Suffix: ", suffix)
	for _, resource := range resources {
		log.New(os.Stderr, "", 0).Println("Identifier: ", *resource.Identifier)
		log.New(os.Stderr, "", 0).Println("Properties: ", *resource.Properties)
	}

	return "", fmt.Errorf("could not find resource identifier for type: %s: %v", resourceType, resources)
}

func (c *ccapiLookups) listResources(
	ctx context.Context,
	resourceType common.ResourceType,
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

func (c *ccapiLookups) GetStackResources(ctx context.Context, stackName common.StackName) error {
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
				ResourceType: common.ResourceType(*s.ResourceType),
				LogicalID:    common.LogicalResourceID(*s.LogicalResourceId),
				PhysicalID:   common.PhysicalResourceID(*s.PhysicalResourceId),
			}
			c.cfnStackResources[r.LogicalID] = r
		}
	}
	return nil
}
