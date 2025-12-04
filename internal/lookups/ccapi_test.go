package lookups

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/ratelimit"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
)

type mockListResourcesPager struct {
	resourceDescriptions []types.ResourceDescription
	typeName             string
	page                 int
	err                  error
	errSequence          []error
	seqIdx               int
}

func (m *mockListResourcesPager) HasMorePages() bool {
	if m.page == 0 {
		m.page++
		return true
	}
	return false
}

func (m *mockListResourcesPager) NextPage(ctx context.Context, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error) {
	if len(m.errSequence) > 0 {
		if m.seqIdx < len(m.errSequence) {
			err := m.errSequence[m.seqIdx]
			m.seqIdx++
			if err != nil {
				return nil, err
			}
		}
	} else if m.err != nil {
		return nil, m.err
	}
	return &cloudcontrol.ListResourcesOutput{
		ResourceDescriptions: m.resourceDescriptions,
		TypeName:             &m.typeName,
	}, nil
}

type mockCCAPIClient struct {
	mockGetPager func(typeName string, resourceModel *string) ListResourcesPager
}

type mockEventsClient struct {
	arn   string
	err   error
	input *eventbridge.DescribeRuleInput
}

func (m *mockEventsClient) DescribeRule(ctx context.Context, params *eventbridge.DescribeRuleInput, optFns ...func(*eventbridge.Options)) (*eventbridge.DescribeRuleOutput, error) {
	m.input = params
	if m.err != nil {
		return nil, m.err
	}
	return &eventbridge.DescribeRuleOutput{
		Arn: aws.String(m.arn),
	}, nil
}

type fixedBackoff time.Duration

func (f fixedBackoff) BackoffDelay(int, error) (time.Duration, error) {
	return time.Duration(f), nil
}

func (m *mockCCAPIClient) GetPager(typeName string, resourceModel *string) ListResourcesPager {
	return m.mockGetPager(typeName, resourceModel)
}

func TestFindPrimaryResourceID(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws-native:s3:Bucket")
		logicalID := common.LogicalResourceID("bucket")
		props := map[string]interface{}{}

		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				return &mockListResourcesPager{
					typeName: "AWS::S3::Bucket",
					resourceDescriptions: []types.ResourceDescription{
						{
							Identifier: aws.String("bucket-name"),
						},
					},
				}
			},
		}

		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"bucket": {
					ResourceType: "AWS::S3::Bucket",
					PhysicalID:   "bucket-name",
					LogicalID:    "bucket",
				},
			},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: map[resourceCacheKey][]types.ResourceDescription{},
		}
		stackResource := ccapiLookups.cfnStackResources

		actual, err := ccapiLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("bucket-name"), actual)
		assert.Equal(t, stackResource, ccapiLookups.cfnStackResources)
	})

	t.Run("events rule custom resolver default bus", func(t *testing.T) {
		ctx := context.Background()
		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Rule": {
					ResourceType: "AWS::Events::Rule",
					PhysicalID:   "default|my-rule",
					LogicalID:    "Rule",
				},
			},
			ccapiClient:        &mockCCAPIClient{},
			ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
			region:             "us-west-2",
			account:            "123456789012",
			eventsClient: &mockEventsClient{
				arn: "arn:aws:events:us-west-2:123456789012:rule/my-rule",
			},
		}
		ccapiLookups.customResolvers = map[common.ResourceType]customResolver{
			"AWS::Events::Rule": ccapiLookups.resolveEventsRule,
		}

		actual, err := ccapiLookups.findOwnNativeId(
			ctx,
			common.ResourceType("AWS::Events::Rule"),
			common.LogicalResourceID("Rule"),
			resource.PropertyKey("Arn"),
		)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("arn:aws:events:us-west-2:123456789012:rule/my-rule"), actual)
	})

	t.Run("events rule custom resolver custom bus", func(t *testing.T) {
		ctx := context.Background()
		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Rule": {
					ResourceType: "AWS::Events::Rule",
					PhysicalID:   "orders|match-order",
					LogicalID:    "Rule",
				},
			},
			ccapiClient:        &mockCCAPIClient{},
			ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
			region:             "us-west-2",
			account:            "123456789012",
			eventsClient: &mockEventsClient{
				arn: "arn:aws:events:us-west-2:123456789012:rule/orders/match-order",
			},
		}
		ccapiLookups.customResolvers = map[common.ResourceType]customResolver{
			"AWS::Events::Rule": ccapiLookups.resolveEventsRule,
		}

		actual, err := ccapiLookups.findOwnNativeId(
			ctx,
			common.ResourceType("AWS::Events::Rule"),
			common.LogicalResourceID("Rule"),
			resource.PropertyKey("Arn"),
		)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("arn:aws:events:us-west-2:123456789012:rule/orders/match-order"), actual)
	})

	t.Run("events rule without composite physical id falls back to lookup", func(t *testing.T) {
		ctx := context.Background()
		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				assert.Equal(t, "AWS::Events::Rule", typeName)
				return &mockListResourcesPager{
					typeName: "AWS::Events::Rule",
					resourceDescriptions: []types.ResourceDescription{
						{
							Identifier: aws.String("arn:aws:events:us-west-2:123456789012:rule/my-rule"),
						},
					},
				}
			},
		}
		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Rule": {
					ResourceType: "AWS::Events::Rule",
					PhysicalID:   "my-rule",
					LogicalID:    "Rule",
				},
			},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
		}
		ccapiLookups.customResolvers = map[common.ResourceType]customResolver{
			"AWS::Events::Rule": ccapiLookups.resolveEventsRule,
		}

		actual, err := ccapiLookups.findOwnNativeId(
			ctx,
			common.ResourceType("AWS::Events::Rule"),
			common.LogicalResourceID("Rule"),
			resource.PropertyKey("Arn"),
		)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("arn:aws:events:us-west-2:123456789012:rule/my-rule"), actual)
	})

	t.Run("arn suffix uses physical id when already arn", func(t *testing.T) {
		ctx := context.Background()
		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				t.Fatalf("expected to skip CCAPI lookup for %s", typeName)
				return nil
			},
		}

		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Topic": {
					ResourceType: "AWS::SNS::Topic",
					PhysicalID:   "arn:aws:sns:us-west-2:123456789012:my-topic",
					LogicalID:    "Topic",
				},
			},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
		}

		actual, err := ccapiLookups.findOwnNativeId(
			ctx,
			common.ResourceType("AWS::SNS::Topic"),
			common.LogicalResourceID("Topic"),
			resource.PropertyKey("TopicArn"),
		)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("arn:aws:sns:us-west-2:123456789012:my-topic"), actual)
	})

	t.Run("arn suffix falls back to lookup when physical id not arn", func(t *testing.T) {
		ctx := context.Background()
		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				assert.Equal(t, "AWS::SNS::Topic", typeName)
				return &mockListResourcesPager{
					typeName: typeName,
					resourceDescriptions: []types.ResourceDescription{
						{
							Identifier: aws.String("arn:aws:sns:us-west-2:123456789012:my-topic"),
						},
					},
				}
			},
		}

		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Topic": {
					ResourceType: "AWS::SNS::Topic",
					PhysicalID:   "my-topic",
					LogicalID:    "Topic",
				},
			},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
		}

		actual, err := ccapiLookups.findOwnNativeId(
			ctx,
			common.ResourceType("AWS::SNS::Topic"),
			common.LogicalResourceID("Topic"),
			resource.PropertyKey("TopicArn"),
		)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("arn:aws:sns:us-west-2:123456789012:my-topic"), actual)
	})

	t.Run("composite id and multiple resources", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws-native:ec2:Route")
		logicalID := common.LogicalResourceID("route1")
		props := map[string]interface{}{
			"RouteTableId": "rtb-1234",
		}

		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				return &mockListResourcesPager{
					typeName: "AWS::EC2::Route",
					resourceDescriptions: []types.ResourceDescription{
						{
							Identifier: aws.String("rtb-1234|0.0.0.0/0"),
						},
						{
							Identifier: aws.String("rtb-1234|10.0.0.0/16"),
						},
					},
				}
			},
		}

		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"route1": {
					ResourceType: "AWS::EC2::Route",
					PhysicalID:   "rtb-1234|0.0.0.0/0",
					LogicalID:    "route1",
				},
				"route2": {
					ResourceType: "AWS::EC2::Route",
					PhysicalID:   "rtb-1234|10.0.0.0/16",
					LogicalID:    "route2",
				},
			},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: map[resourceCacheKey][]types.ResourceDescription{},
		}

		actual, err := ccapiLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("rtb-1234|0.0.0.0/0"), actual)
		assert.Equal(t, ccapiLookups.cfnStackResources[common.LogicalResourceID("route1")].Props, props)
	})

	t.Run("error rendering resource model", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws-native:ec2:Route")
		logicalID := common.LogicalResourceID("route1")
		props := map[string]interface{}{
			"RouteTableId": []string{"rtb-1234"}, // invalid type
		}

		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				return &mockListResourcesPager{}
			},
		}

		ccapiLookups := &ccapiLookups{
			cfnStackResources:  map[common.LogicalResourceID]CfnStackResource{},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: map[resourceCacheKey][]types.ResourceDescription{},
		}

		_, err := ccapiLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.ErrorContains(t, err, "expected id property \"RouteTableId\" to be a string")
	})

	t.Run("Missing required property fallthrough", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws-native:elasticloadbalancingv2:Listener")
		logicalID := common.LogicalResourceID("Listener")
		props := map[string]interface{}{
			"LoadBalancerArn": "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-load-balancer/50dc6c495c0c9188",
		}

		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				if resourceModel == nil {
					return &mockListResourcesPager{
						typeName: "AWS::ElasticLoadBalancingV2::Listener",
						err: &types.InvalidRequestException{
							Message: aws.String("Missing or Invalid ResourceModel...Required property: [LoadBalancerArn]"),
						},
					}
				} else {
					return &mockListResourcesPager{
						typeName: "AWS::ElasticLoadBalancingV2::Listener",
						resourceDescriptions: []types.ResourceDescription{
							{
								Identifier: aws.String("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/my-load-balancer/50dc6c495c0c9188/0467ef3c8400ae65"),
							},
						},
					}
				}
			},
		}

		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Listener": {
					ResourceType: "AWS::ElasticLoadBalancingV2::Listener",
					PhysicalID:   "arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/my-load-balancer/50dc6c495c0c9188/0467ef3c8400ae65",
					LogicalID:    "Listener",
					Props: map[string]interface{}{
						"LoadBalancerArn": "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-load-balancer/50dc6c495c0c9188",
					},
				},
			},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: map[resourceCacheKey][]types.ResourceDescription{},
		}

		actual, err := ccapiLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/my-load-balancer/50dc6c495c0c9188/0467ef3c8400ae65"), actual)
	})

	t.Run("ScalingPolicy uses composite identifier with retry", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws-native:applicationautoscaling:ScalingPolicy")
		logicalID := common.LogicalResourceID("ScalingPolicy")
		props := map[string]interface{}{
			"PolicyName":        "MyPolicy",
			"ResourceId":        "service/myCluster/myService",
			"ScalableDimension": "ecs:service:DesiredCount",
		}

		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				if resourceModel != nil {
					var model map[string]string
					_ = json.Unmarshal([]byte(*resourceModel), &model)

					// First call: Only ScalableDimension (Arn is not in props, it's an output)
					if len(model) == 1 && model["ScalableDimension"] == "ecs:service:DesiredCount" {
						return &mockListResourcesPager{
							typeName: "AWS::ApplicationAutoScaling::ScalingPolicy",
							err: &types.InvalidRequestException{
								Message: aws.String("InvalidRequestException: Missing or invalid ResourceModel property... Required property: (#: required key [ServiceNamespace] not found)"),
							},
						}
					}

					// Second call: Only ServiceNamespace (the missing property extracted from error)
					if len(model) == 1 && model["ServiceNamespace"] == "ecs" {
						return &mockListResourcesPager{
							typeName: "AWS::ApplicationAutoScaling::ScalingPolicy",
							resourceDescriptions: []types.ResourceDescription{
								{
									Identifier: aws.String("arn:aws:autoscaling:us-west-2:123456789012:scalingPolicy:uuid:autoScalingGroupName/groupName:policyName/MyPolicy|ecs:service:DesiredCount"),
								},
							},
						}
					}
				}
				return &mockListResourcesPager{typeName: "AWS::ApplicationAutoScaling::ScalingPolicy"}
			},
		}

		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"ScalingPolicy": {
					ResourceType: "AWS::ApplicationAutoScaling::ScalingPolicy",
					PhysicalID:   "arn:aws:autoscaling:us-west-2:123456789012:scalingPolicy:uuid:autoScalingGroupName/groupName:policyName/MyPolicy|ecs:service:DesiredCount",
					LogicalID:    "ScalingPolicy",
					Props:        props,
				},
			},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
		}

		actual, err := ccapiLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("arn:aws:autoscaling:us-west-2:123456789012:scalingPolicy:uuid:autoScalingGroupName/groupName:policyName/MyPolicy|ecs:service:DesiredCount"), actual)
	})

	t.Run("BucketPolicy uses physical ID strategy", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws-native:s3:BucketPolicy")
		logicalID := common.LogicalResourceID("BucketPolicy")
		props := map[string]interface{}{
			"Bucket": "my-bucket",
			"PolicyDocument": map[string]interface{}{
				"Statement": []interface{}{},
			},
		}

		// Mock client shouldn't be called because we use StrategyPhysicalID
		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				t.Fatal("Should not call CCAPI for BucketPolicy")
				return nil
			},
		}

		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"BucketPolicy": {
					ResourceType: "AWS::S3::BucketPolicy",
					PhysicalID:   "my-bucket", // Physical ID is the bucket name
					LogicalID:    "BucketPolicy",
					Props:        props,
				},
			},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
		}

		actual, err := ccapiLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("my-bucket"), actual)
	})

	t.Run("Missing required property fallthrough - new format", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws-native:lambda:Permission")
		logicalID := common.LogicalResourceID("Permission")
		props := map[string]interface{}{
			"FunctionName": "my-function",
		}

		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				if resourceModel == nil {
					return &mockListResourcesPager{
						typeName: "AWS::Lambda::Permission",
						err: &types.InvalidRequestException{
							Message: aws.String("InvalidRequestException: Missing or invalid ResourceModel property in AWS::Lambda::Permission list handler request input.Required property:  (#: required key [FunctionName] not found)"),
						},
					}
				} else {
					return &mockListResourcesPager{
						typeName: "AWS::Lambda::Permission",
						resourceDescriptions: []types.ResourceDescription{
							{
								Identifier: aws.String("my-function/permission-id"),
							},
						},
					}
				}
			},
		}

		ccapiLookups := &ccapiLookups{
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Permission": {
					ResourceType: "AWS::Lambda::Permission",
					PhysicalID:   "my-function/permission-id",
					LogicalID:    "Permission",
					Props: map[string]interface{}{
						"FunctionName": "my-function",
					},
				},
			},
			ccapiClient:        ccapiClient,
			ccapiResourceCache: map[resourceCacheKey][]types.ResourceDescription{},
		}

		actual, err := ccapiLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("my-function/permission-id"), actual)
	})

	t.Run("listResources retries on throttling", func(t *testing.T) {
		ctx := context.Background()
		oldRetryer := newCCAPIRetryer
		newCCAPIRetryer = func() aws.Retryer {
			return retry.NewAdaptiveMode(func(o *retry.AdaptiveModeOptions) {
				o.StandardOptions = append(o.StandardOptions, func(so *retry.StandardOptions) {
					so.MaxAttempts = 3
					so.RateLimiter = ratelimit.None
					so.Backoff = fixedBackoff(time.Millisecond)
				})
			})
		}
		defer func() {
			newCCAPIRetryer = oldRetryer
		}()

		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				return &mockListResourcesPager{
					typeName: "AWS::S3::Bucket",
					errSequence: []error{
						&types.ThrottlingException{Message: aws.String("slow down")},
						nil,
					},
					resourceDescriptions: []types.ResourceDescription{
						{Identifier: aws.String("bucket-name")},
					},
				}
			},
		}

		ccapiLookups := &ccapiLookups{
			ccapiClient:        ccapiClient,
			ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
		}

		res, err := ccapiLookups.listResources(ctx, common.ResourceType("AWS::S3::Bucket"), map[string]string{})
		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, "bucket-name", *res[0].Identifier)
	})

	t.Run("listResources stops after max throttling", func(t *testing.T) {
		ctx := context.Background()
		oldRetryer := newCCAPIRetryer
		newCCAPIRetryer = func() aws.Retryer {
			return retry.NewAdaptiveMode(func(o *retry.AdaptiveModeOptions) {
				o.StandardOptions = append(o.StandardOptions, func(so *retry.StandardOptions) {
					so.MaxAttempts = 2
					so.RateLimiter = ratelimit.None
					so.Backoff = fixedBackoff(time.Millisecond)
				})
			})
		}
		defer func() {
			newCCAPIRetryer = oldRetryer
		}()

		ccapiClient := &mockCCAPIClient{
			mockGetPager: func(typeName string, resourceModel *string) ListResourcesPager {
				return &mockListResourcesPager{
					typeName: "AWS::S3::Bucket",
					errSequence: []error{
						&types.ThrottlingException{Message: aws.String("slow down")},
						&types.ThrottlingException{Message: aws.String("still slow")},
					},
				}
			},
		}

		ccapiLookups := &ccapiLookups{
			ccapiClient:        ccapiClient,
			ccapiResourceCache: make(map[resourceCacheKey][]types.ResourceDescription),
		}

		_, err := ccapiLookups.listResources(ctx, common.ResourceType("AWS::S3::Bucket"), map[string]string{})
		assert.Error(t, err)
	})
}
