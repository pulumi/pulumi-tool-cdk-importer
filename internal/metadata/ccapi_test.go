package metadata

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
)

func TestAwsNativeMetadataSourcePrimaryIdentifierOverride(t *testing.T) {
	src := NewCCApiMetadataSource()

	t.Run("returns override for Lambda Permission", func(t *testing.T) {
		t.Skip("Removed the override for Lambda Permissions, update if we need another override")
		props, ok := src.PrimaryIdentifier(tokens.Type("aws-native:lambda:Permission"))
		assert.True(t, ok)
		assert.Equal(t, []resource.PropertyKey{
			resource.PropertyKey("functionArn"),
			resource.PropertyKey("id"),
		}, props)
	})

	t.Run("falls back to JSON metadata for resources without override", func(t *testing.T) {
		// Test with a resource that exists in JSON but has no override
		props, ok := src.PrimaryIdentifier(tokens.Type("aws-native:s3:Bucket"))
		assert.True(t, ok)
		// Should return whatever is in the JSON metadata
		assert.NotNil(t, props)
		assert.Greater(t, len(props), 0)
	})

	t.Run("returns false for unknown resources", func(t *testing.T) {
		_, ok := src.PrimaryIdentifier(tokens.Type("aws-native:unknown:Resource"))
		assert.False(t, ok)
	})
}

func TestAwsNativeMetadataSourceListHandlerSchema(t *testing.T) {
	src := NewCCApiMetadataSource()

	required := src.ListHandlerRequiredProperties(common.ResourceType("AWS::Amplify::Branch"))
	assert.NotNil(t, required)
	assert.Contains(t, required, "AppId")
}
