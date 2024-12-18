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
			"ApiId": "apiId",
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
		assert.Equal(t, common.PrimaryResourceID("stageId"), actual)
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
}
