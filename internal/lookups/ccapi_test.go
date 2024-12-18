package lookups

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
)

type mockListResourcesPager struct {
	resourceDescriptions []types.ResourceDescription
	typeName             string
	page                 int
	err                  error
}

func (m *mockListResourcesPager) HasMorePages() bool {
	if m.page == 0 {
		m.page++
		return true
	}
	return false
}

func (m *mockListResourcesPager) NextPage(ctx context.Context, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error) {
	if m.err != nil {
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
}
