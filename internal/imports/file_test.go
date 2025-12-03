package imports

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
)

func TestBuildImportFileBasicBucket(t *testing.T) {
	l := &lookups.Lookups{
		Region:  "us-west-2",
		Account: "123456789012",
		CfnStackResources: map[common.LogicalResourceID]lookups.CfnStackResource{
			common.LogicalResourceID("Bucket"): {
				ResourceType: "AWS::S3::Bucket",
				LogicalID:    common.LogicalResourceID("Bucket"),
				PhysicalID:   common.PhysicalResourceID("my-bucket"),
			},
		},
	}

	file, summary, err := BuildImportFile(context.Background(), l)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.EmittedResources)
	assert.Len(t, file.Resources, 1)

	resource := file.Resources[0]
	assert.Equal(t, "aws:s3/bucket:Bucket", resource.Type)
	assert.Equal(t, "Bucket", resource.Name)
	assert.Equal(t, "my-bucket", resource.ID)
	assert.Equal(t, "Bucket", resource.LogicalName)
	assert.Empty(t, summary.PlaceholderEntries)
}

func TestBuildImportFilePlaceholderWhenIdUnknown(t *testing.T) {
	l := &lookups.Lookups{
		Region:  "us-west-2",
		Account: "123456789012",
		CfnStackResources: map[common.LogicalResourceID]lookups.CfnStackResource{
			common.LogicalResourceID("Route"): {
				ResourceType: "AWS::ApiGatewayV2::Route",
				LogicalID:    common.LogicalResourceID("Route"),
				PhysicalID:   "",
			},
		},
	}

	file, summary, err := BuildImportFile(context.Background(), l)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.EmittedResources)
	assert.Len(t, summary.PlaceholderEntries, 1)

	if assert.Len(t, file.Resources, 1) {
		resource := file.Resources[0]
		assert.Equal(t, "aws:apigatewayv2/route:Route", resource.Type)
		assert.Equal(t, placeholderID, resource.ID)
	}
}

func TestBuildImportFileSkipsUnsupportedTypes(t *testing.T) {
	l := &lookups.Lookups{
		Region:  "us-west-2",
		Account: "123456789012",
		CfnStackResources: map[common.LogicalResourceID]lookups.CfnStackResource{
			common.LogicalResourceID("Metadata"): {
				ResourceType: "AWS::CDK::Metadata",
				LogicalID:    common.LogicalResourceID("Metadata"),
			},
		},
	}

	file, summary, err := BuildImportFile(context.Background(), l)
	require.NoError(t, err)
	assert.Equal(t, 0, summary.EmittedResources)
	assert.Len(t, summary.SkippedResources, 1)
	assert.Empty(t, file.Resources)
}
