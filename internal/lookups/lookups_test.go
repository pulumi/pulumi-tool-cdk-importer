package lookups

import (
	"testing"

	providerMetadata "github.com/pulumi/pulumi-aws-native/provider/pkg/metadata"
	"github.com/pulumi/pulumi-aws-native/provider/pkg/naming"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
)

type mockMetadataSource struct {
	resources map[string]providerMetadata.CloudAPIResource
}

func (m *mockMetadataSource) ResourceType(resourceToken tokens.Type) (common.ResourceType, bool) {
	res, ok := m.resources[string(resourceToken)]
	if !ok {
		return "", false
	}
	return common.ResourceType(res.CfType), ok
}

func (m *mockMetadataSource) ResourceToken(resourceType common.ResourceType) (tokens.Type, bool) {
	panic("implement me")
}

func (m *mockMetadataSource) PrimaryIdentifier(resourceToken tokens.Type) ([]resource.PropertyKey, bool) {
	res, ok := m.resources[string(resourceToken)]
	if !ok {
		return nil, false
	}
	props := []resource.PropertyKey{}
	for _, rawProp := range res.PrimaryIdentifier {
		prop := resource.PropertyKey(rawProp)
		props = append(props, prop)
	}
	return props, true
}

func (m *mockMetadataSource) Resource(resourceToken string) (providerMetadata.CloudAPIResource, error) {
	panic("implement me")
}

func (m *mockMetadataSource) Separator(resourceToken tokens.Type) string {
	return "/"
}

func Test_renderResourceModel(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		actual, err := renderResourceModel(
			[]resource.PropertyKey{
				resource.PropertyKey("ApiId"),
				resource.PropertyKey("StageName"),
			},
			map[string]interface{}{
				"ApiId": "1234",
			},
			func(s string) string { return s },
		)
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{
			"ApiId": "1234",
		}, actual)
	})

	t.Run("with name transform", func(t *testing.T) {
		actual, err := renderResourceModel(
			[]resource.PropertyKey{
				resource.PropertyKey("apiId"),
				resource.PropertyKey("stageName"),
			},
			map[string]interface{}{
				"ApiId": "1234",
			},
			func(s string) string { return naming.ToCfnName(s, map[string]string{}) },
		)
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{
			"ApiId": "1234",
		}, actual)
	})
}

func Test_getPrimaryIdentifiers(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		resType, parts, err := getPrimaryIdentifiers(&mockMetadataSource{
			resources: map[string]providerMetadata.CloudAPIResource{
				"AWS::ApiGatewayV2::Api": {
					CfType: "AWS::ApiGatewayV2::Api",
					PrimaryIdentifier: []string{
						"ApiId",
						"StageName",
					},
				},
			},
		}, tokens.Type("AWS::ApiGatewayV2::Api"))
		assert.NoError(t, err)
		assert.Equal(t, common.ResourceType("AWS::ApiGatewayV2::Api"), resType)
		assert.Equal(t, []resource.PropertyKey{
			resource.PropertyKey("ApiId"),
			resource.PropertyKey("StageName"),
		}, parts)
	})

	t.Run("could not find resource type", func(t *testing.T) {
		_, _, err := getPrimaryIdentifiers(&mockMetadataSource{
			resources: map[string]providerMetadata.CloudAPIResource{
				"AWS::ApiGatewayV2::Api": {
					CfType: "AWS::ApiGatewayV2::Api",
					PrimaryIdentifier: []string{
						"ApiId",
						"StageName",
					},
				},
			},
		}, tokens.Type("AWS::ApiGatewayV2::Stage"))
		assert.ErrorContains(t, err, "Unknown resource type")
	})
}

func Test_findLogicalResourceID(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		urn := resource.URN("urn:pulumi:stack::project::aws:apigatewayv2/stage:Stage::stage")
		cfnStackResources := map[common.LogicalResourceID]CfnStackResource{
			"stage": {
				LogicalID:    "Stage",
				ResourceType: "AWS::ApiGatewayV2::Stage",
			},
		}
		actual, err := findLogicalResourceID(urn, &mockMetadataSource{
			resources: map[string]providerMetadata.CloudAPIResource{
				"aws:apigatewayv2/stage:Stage": {
					CfType: "AWS::ApiGatewayV2::Stage",
				},
			}}, cfnStackResources)
		assert.NoError(t, err)
		assert.Equal(t, common.LogicalResourceID("Stage"), actual)
	})

	t.Run("too many matches", func(t *testing.T) {
		urn := resource.URN("urn:pulumi:stack::project::aws:apigatewayv2/stage:Stage::stage")
		cfnStackResources := map[common.LogicalResourceID]CfnStackResource{
			"stage": {
				LogicalID:    "Stage",
				ResourceType: "AWS::ApiGatewayV2::Stage",
			},
			"otherResource": {
				LogicalID:    "OtherStage",
				ResourceType: "AWS::ApiGatewayV2::Stage",
			},
		}
		_, err := findLogicalResourceID(urn, &mockMetadataSource{
			resources: map[string]providerMetadata.CloudAPIResource{
				"aws:apigatewayv2/stage:Stage": {
					CfType: "AWS::ApiGatewayV2::Stage",
				},
			}}, cfnStackResources)
		assert.ErrorContains(t, err, "Conflicting matching CF resources")
	})

	t.Run("no matches", func(t *testing.T) {
		urn := resource.URN("urn:pulumi:stack::project::aws:apigatewayv2/stage:Stage::stage")
		cfnStackResources := map[common.LogicalResourceID]CfnStackResource{
			"stage": {
				LogicalID:    "Other",
				ResourceType: "AWS::ApiGatewayV2::Stage",
			},
			"otherResource": {
				LogicalID:    "Resource",
				ResourceType: "AWS::ApiGatewayV2::Stage",
			},
		}
		_, err := findLogicalResourceID(urn, &mockMetadataSource{
			resources: map[string]providerMetadata.CloudAPIResource{
				"aws:apigatewayv2/stage:Stage": {
					CfType: "AWS::ApiGatewayV2::Stage",
				},
			}}, cfnStackResources)
		assert.ErrorContains(t, err, "No matching CF resources")
	})
}
