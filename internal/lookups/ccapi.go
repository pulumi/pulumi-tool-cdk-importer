package lookups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/ratelimit"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
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

// retryer factory (overridable in tests)
var newCCAPIRetryer = func() aws.Retryer {
	return retry.NewAdaptiveMode(func(o *retry.AdaptiveModeOptions) {
		o.StandardOptions = append(o.StandardOptions, func(so *retry.StandardOptions) {
			// Allow more attempts than SDK default and let adaptive mode sleep for throttling
			so.MaxAttempts = 8
			so.RateLimiter = ratelimit.None
		})
	})
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
		// TODO: debug logging
		// fmt.Printf("Rendering Resource Models for %s - %s: Parts: %v: Props: %v", resourceType, logicalID, idParts, props)
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
	md := metadata.NewCCApiMetadataSource()
	strategy := md.GetIdPropertyStrategy(resourceType, idPropertyName)

	// 1. Check for explicit strategy override
	if strategy == metadata.StrategyPhysicalID {
		if r, ok := c.cfnStackResources[logicalID]; ok {
			// NOTE! Assuming that PrimaryResourceID matches the PhysicalID.
			return common.PrimaryResourceID(r.PhysicalID), nil
		}
		return "", fmt.Errorf("Resource doesn't exist in this stack which isn't possible!")
	} else if strategy == metadata.StrategyLookup {
		if r, ok := c.cfnStackResources[logicalID]; ok {
			suffix := string(r.PhysicalID)
			id, err := c.findResourceIdentifier(ctx, resourceType, logicalID, suffix, nil)
			if err != nil {
				return "", fmt.Errorf("Could not find id for %s: %w", logicalID, err)
			}
			return id, nil
		}
	}

	// 2. ARN heuristic: properties ending in 'arn' typically need lookup
	if strings.HasSuffix(idPropertyName, "arn") {
		if r, ok := c.cfnStackResources[logicalID]; ok {
			suffix := string(r.PhysicalID)
			id, err := c.findResourceIdentifier(ctx, resourceType, logicalID, suffix, nil)
			if err != nil {
				return "", fmt.Errorf("Could not find id for %s: %w", logicalID, err)
			}
			return id, nil
		}
	}

	// 3. Default: assume PhysicalID matches the primary identifier
	if r, ok := c.cfnStackResources[logicalID]; ok {
		return common.PrimaryResourceID(r.PhysicalID), nil
	}
	return "", fmt.Errorf("Resource doesn't exist in this stack which isn't possible!")
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
	var err error
	resourceModel, err = c.applyListHandlerResourceModel(resourceType, logicalID, resourceModel)
	if err != nil {
		return "", fmt.Errorf("could not build list handler resource model for %s: %w", logicalID, err)
	}
	resources, err := c.listResources(ctx, resourceType, resourceModel)
	if err != nil {
		fmt.Printf("Error listing resource: %s, %v, %v\n", resourceType, resourceModel, err)
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
			// We attempt to match against multiple regexes as the error message format is not consistent
			// across all resources.
			regexes := []*regexp.Regexp{
				regexp.MustCompile(`Required property: \[(.*?)\]`),
				regexp.MustCompile(`required key \[(.*?)\]`),
			}

			var missingProperty string
			for _, re := range regexes {
				match := re.FindStringSubmatch(invalid.ErrorMessage())
				if len(match) > 1 {
					missingProperty = match[1]
					break
				}
			}

			if missingProperty != "" {
				fmt.Printf("Found missing property for %s %s: %s", resourceType, logicalID, missingProperty)
				// create a correct resource model with the missing property
				resourceModel, err = renderResourceModel([]resource.PropertyKey{
					resource.PropertyKey(missingProperty),
				}, c.cfnStackResources[logicalID].Props, func(s string) string {
					return s
				})
				if err != nil {
					return "", fmt.Errorf("Error rendering resource model: %w", err)
				}
				if len(resourceModel) == 0 {
					if derived := deriveMissingProperty(resourceType, missingProperty, c.cfnStackResources[logicalID].Props); len(derived) > 0 {
						resourceModel = derived
					} else {
						return "", fmt.Errorf("Error finding resource of type %s with resourceModel: %v Props: %v: MissingProperty %s", resourceType, resourceModel, c.cfnStackResources[logicalID].Props, missingProperty)
					}
				}
				// run it again with the new resource model
				return c.findResourceIdentifier(ctx, resourceType, logicalID, suffix, resourceModel)
			}
			return "", fmt.Errorf("Error finding resource of type %s with resourceModel: %v Props: %v: MissingProperty %s:  %w", resourceType, resourceModel, c.cfnStackResources[logicalID].Props, missingProperty, err)
		} else {
			return "", fmt.Errorf("Unknown error: Error finding resource of type %s with resourceModel: %v Props: %v: %w", resourceType, resourceModel, c.cfnStackResources[logicalID].Props, err)
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

	return "", fmt.Errorf("could not find resource identifier for type: %s: %v", resourceType, resourceModel)
}

func (c *ccapiLookups) applyListHandlerResourceModel(
	resourceType common.ResourceType,
	logicalID common.LogicalResourceID,
	resourceModel map[string]string,
) (map[string]string, error) {
	md := metadata.NewCCApiMetadataSource()
	required := md.ListHandlerRequiredProperties(resourceType)
	if len(required) == 0 {
		return resourceModel, nil
	}

	mergedModel := make(map[string]string, len(resourceModel)+len(required))
	for k, v := range resourceModel {
		mergedModel[k] = v
	}

	missing := []resource.PropertyKey{}
	for _, prop := range required {
		if _, ok := mergedModel[prop]; ok {
			continue
		}
		missing = append(missing, resource.PropertyKey(prop))
	}

	if len(missing) == 0 {
		return mergedModel, nil
	}

	stackResource, ok := c.cfnStackResources[logicalID]
	if !ok {
		return mergedModel, nil
	}

	derived, err := renderResourceModel(missing, stackResource.Props, func(s string) string { return s })
	if err != nil {
		return nil, err
	}

	for k, v := range derived {
		mergedModel[k] = v
	}

	return mergedModel, nil
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
	retryer := newCCAPIRetryer()
	releaseInitial := retryer.GetInitialToken()
	defer func() {
		_ = releaseInitial(nil)
	}()

	for paginator.HasMorePages() {
		var output *cloudcontrol.ListResourcesOutput
		var err error

		for attempt := 1; attempt <= retryer.MaxAttempts(); attempt++ {
			output, err = paginator.NextPage(ctx)
			if err == nil {
				break
			}

			sdkErr := toAPIError(err)
			if !retryer.IsErrorRetryable(sdkErr) || attempt >= retryer.MaxAttempts() {
				return nil, err
			}

			releaseRetryToken, tokenErr := retryer.GetRetryToken(ctx, sdkErr)
			if tokenErr != nil {
				return nil, err
			}

			delay, delayErr := retryer.RetryDelay(attempt, sdkErr)
			if delayErr != nil {
				_ = releaseRetryToken(sdkErr)
				return nil, err
			}

			select {
			case <-ctx.Done():
				_ = releaseRetryToken(sdkErr)
				return nil, ctx.Err()
			case <-time.After(delay):
				_ = releaseRetryToken(nil)
			}
		}

		resources = append(resources, output.ResourceDescriptions...)
	}

	c.ccapiResourceCache[cacheKey] = resources
	return resources, nil
}

func isThrottlingError(err error) bool {
	var throttling *types.ThrottlingException
	if errors.As(err, &throttling) {
		return true
	}
	if apiErr, ok := err.(smithy.APIError); ok {
		code := apiErr.ErrorCode()
		if strings.Contains(strings.ToLower(code), "throttling") {
			return true
		}
	}
	return false
}

func toAPIError(err error) error {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}

	code := "GeneralServiceException"
	if isThrottlingError(err) {
		code = "ThrottlingException"
	}

	return &smithy.GenericAPIError{
		Code:    code,
		Message: err.Error(),
	}
}

func deriveMissingProperty(resourceType common.ResourceType, missingProperty string, props map[string]any) map[string]string {
	if resourceType == "AWS::ApplicationAutoScaling::ScalingPolicy" && strings.EqualFold(missingProperty, "ServiceNamespace") {
		if sd, ok := props["ScalableDimension"].(string); ok {
			parts := strings.SplitN(sd, ":", 2)
			if len(parts) > 0 && parts[0] != "" {
				return map[string]string{"ServiceNamespace": parts[0]}
			}
		}
		if stid, ok := props["ScalingTargetId"].(string); ok {
			segments := strings.Split(stid, "|")
			if len(segments) > 0 {
				candidate := segments[len(segments)-1]
				candidate = strings.TrimSpace(candidate)
				if candidate != "" {
					return map[string]string{"ServiceNamespace": candidate}
				}
			}
		}
	}
	return nil
}
