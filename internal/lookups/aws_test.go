package lookups

import (
	"context"
	"testing"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
)

func TestFindAwsPrimaryResourceID(t *testing.T) {
	t.Run("apigateway stage", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws:apigatewayv2/stage:Stage")
		logicalID := common.LogicalResourceID("Stage")
		props := map[string]interface{}{
			"apiId": "apiId",
		}

		awsLookups := &awsLookups{
			region:  "us-west-2",
			account: "123456789012",
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Stage": {
					ResourceType: "AWS::ApiGatewayV2::Stage",
					PhysicalID:   "stageId",
					LogicalID:    "Stage",
				},
			},
		}

		actual, err := awsLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("apiId/stageId"), actual)
	})

	t.Run("iam policy", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws:iam/policy:Policy")
		logicalID := common.LogicalResourceID("Policy")
		props := map[string]interface{}{}

		awsLookups := &awsLookups{
			region:  "us-west-2",
			account: "123456789012",
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Policy": {
					ResourceType: "AWS::IAM::Policy",
					PhysicalID:   "Policy",
					LogicalID:    "Policy",
				},
			},
		}

		actual, err := awsLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("arn:aws:iam::123456789012:policy/Policy"), actual)
	})

	t.Run("iam role policy with colon separator", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws:iam/rolePolicy:RolePolicy")
		logicalID := common.LogicalResourceID("RolePolicy")
		props := map[string]interface{}{
			"role": "MyRole",
		}

		awsLookups := &awsLookups{
			region:  "us-west-2",
			account: "123456789012",
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"RolePolicy": {
					ResourceType: "AWS::IAM::Policy",
					PhysicalID:   "MyPolicy",
					LogicalID:    "RolePolicy",
				},
			},
		}

		actual, err := awsLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		// Should use colon separator for RolePolicy
		assert.Equal(t, common.PrimaryResourceID("MyRole:MyPolicy"), actual)
	})

	t.Run("service discovery private dns namespace keeps id order", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws:servicediscovery/privateDnsNamespace:PrivateDnsNamespace")
		logicalID := common.LogicalResourceID("Namespace")
		props := map[string]interface{}{
			"vpc": "vpc-02c046362e72bd9be",
		}

		awsLookups := &awsLookups{
			region:  "us-west-2",
			account: "123456789012",
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"Namespace": {
					ResourceType: "AWS::ServiceDiscovery::PrivateDnsNamespace",
					PhysicalID:   "ns-gwftkj6fpvjfzc7n",
					LogicalID:    "Namespace",
				},
			},
		}

		actual, err := awsLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID("ns-gwftkj6fpvjfzc7n:vpc-02c046362e72bd9be"), actual)
	})

	t.Run("queue policy uses provided queueUrl", func(t *testing.T) {
		ctx := context.Background()
		resourceToken := tokens.Type("aws:sqs/queuePolicy:QueuePolicy")
		logicalID := common.LogicalResourceID("QueuePolicy")
		queueURL := "https://sqs.us-west-2.amazonaws.com/123456789012/my-queue"
		props := map[string]interface{}{
			"queueUrl": queueURL,
		}

		awsLookups := &awsLookups{
			region:  "us-west-2",
			account: "123456789012",
			cfnStackResources: map[common.LogicalResourceID]CfnStackResource{
				"QueuePolicy": {
					ResourceType: "AWS::SQS::QueuePolicy",
					PhysicalID:   "policy-physical-id",
					LogicalID:    "QueuePolicy",
				},
			},
		}

		actual, err := awsLookups.FindPrimaryResourceID(ctx, resourceToken, logicalID, props)
		assert.NoError(t, err)
		assert.Equal(t, common.PrimaryResourceID(queueURL), actual)
	})
}
