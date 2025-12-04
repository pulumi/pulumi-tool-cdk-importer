package lookups

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type Lookups struct {
	CCAPIClient       *cloudcontrol.Client
	CfnClient         *cloudformation.Client
	Region            string
	Account           string
	CfnStackResources map[common.LogicalResourceID]CfnStackResource
	EventsClient      *eventbridge.Client
}

func NewDefaultLookups(ctx context.Context) (*Lookups, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfnClient := cloudformation.NewFromConfig(cfg)
	stsClient := sts.NewFromConfig(cfg)
	res, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, err
	}
	return &Lookups{
		CCAPIClient:       cloudcontrol.NewFromConfig(cfg),
		Region:            cfg.Region,
		Account:           *res.Account,
		CfnClient:         cfnClient,
		CfnStackResources: make(map[common.LogicalResourceID]CfnStackResource),
		EventsClient:      eventbridge.NewFromConfig(cfg),
	}, nil
}

// cfnStackResource represents a CloudFormation resource
type CfnStackResource struct {
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
				return nil, fmt.Errorf("expected id property %q to be a string; got %v", cfnName, prop)
			}
			continue
		}
		// Fallback to original key casing if the transformed name is absent.
		if prop, ok := props[string(part)]; ok {
			if val, ok := prop.(string); ok {
				model[cfnName] = val
			} else {
				return nil, fmt.Errorf("expected id property %q to be a string; got %v", cfnName, prop)
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
	cfnStackResources map[common.LogicalResourceID]CfnStackResource,
) (common.LogicalResourceID, error) {
	resourceToken := urn.Type()
	resourceType, ok := metadata.ResourceType(resourceToken)
	if !ok {
		return "", fmt.Errorf("Unknown resource type: %v", resourceToken)
	}

	matchCount := 0
	var match CfnStackResource
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

// GetStackResources Gets all the resources from a CloudFormation stack
func (l *Lookups) GetStackResources(ctx context.Context, stackName common.StackName) error {
	sn := string(stackName)
	paginator := cloudformation.NewListStackResourcesPaginator(l.CfnClient, &cloudformation.ListStackResourcesInput{
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
			r := CfnStackResource{
				ResourceType: common.ResourceType(*s.ResourceType),
				LogicalID:    common.LogicalResourceID(*s.LogicalResourceId),
				PhysicalID:   common.PhysicalResourceID(*s.PhysicalResourceId),
			}
			l.CfnStackResources[r.LogicalID] = r
		}
	}
	return nil
}
