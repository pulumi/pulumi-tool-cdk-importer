package metadata

import (
	"testing"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
)

func TestAwsClassicMetadataSourceSeparator(t *testing.T) {
	src := NewAwsMetadataSource()

	t.Run("returns colon separator derived from format", func(t *testing.T) {
		sep := src.Separator(tokens.Type("aws:appsync/apiKey:ApiKey"))
		assert.Equal(t, ":", sep)
	})

	t.Run("returns default slash for resources without custom separator", func(t *testing.T) {
		sep := src.Separator(tokens.Type("aws:apigatewayv2/stage:Stage"))
		assert.Equal(t, "/", sep)
	})

	t.Run("returns default slash for unknown resources", func(t *testing.T) {
		sep := src.Separator(tokens.Type("aws:unknown/resource:Resource"))
		assert.Equal(t, "/", sep)
	})
}

func TestAwsClassicMetadataPrimaryIdentifierFromSchema(t *testing.T) {
	src := NewAwsMetadataSource()

	props, ok := src.PrimaryIdentifier(tokens.Type("aws:iam/policy:Policy"))
	assert.True(t, ok)
	assert.Equal(t, []resource.PropertyKey{"arn"}, props)
}

func TestAwsClassicMetadataPrefersSpecificMappings(t *testing.T) {
	src := NewAwsMetadataSource()

	resourceType, ok := src.ResourceType(tokens.Type("aws:apigatewayv2/stage:Stage"))
	assert.True(t, ok)
	assert.Equal(t, common.ResourceType("AWS::ApiGatewayV2::Stage"), resourceType)
}
