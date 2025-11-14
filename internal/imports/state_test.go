package imports

import (
	"encoding/json"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFileFromDeploymentMergesStateAndCapture(t *testing.T) {
	t.Parallel()

	stackURN := resource.URN("urn:pulumi:dev::proj::pulumi:pulumi:Stack::proj-dev")
	componentURN := resource.URN("urn:pulumi:dev::proj::custom:component:Thing::component")
	providerURN := resource.URN("urn:pulumi:dev::proj::pulumi:providers:aws::default")
	bucketURN := resource.URN("urn:pulumi:dev::proj::aws:s3/bucket:Bucket::bucket")

	deployment := apitype.DeploymentV3{
		Resources: []apitype.ResourceV3{
			{URN: stackURN, Type: tokens.Type("pulumi:pulumi:Stack")},
			{URN: componentURN, Type: tokens.Type("my:component:Thing"), Parent: stackURN},
			{
				URN:     providerURN,
				Type:    tokens.Type("pulumi:providers:aws"),
				Custom:  true,
				Parent:  stackURN,
				Inputs:  map[string]any{"version": "7.11.0"},
				Outputs: map[string]any{"version": "7.11.0"},
			},
			{
				URN:      bucketURN,
				Type:     tokens.Type("aws:s3/bucket:Bucket"),
				Custom:   true,
				ID:       resource.ID("bucket-123"),
				Parent:   componentURN,
				Provider: string(providerURN),
			},
		},
	}
	bytes, err := json.Marshal(deployment)
	require.NoError(t, err)

	captures := []CaptureMetadata{{
		Type:        "aws:s3/bucket:Bucket",
		Name:        "bucket",
		LogicalName: "MyBucket",
		Properties:  []string{"tags"},
	}}

	file, err := BuildFileFromDeployment(apitype.UntypedDeployment{Deployment: bytes}, captures)
	require.NoError(t, err)
	require.NotNil(t, file.NameTable)
	assert.Equal(t, string(bucketURN), file.NameTable["bucket"])
	assert.Equal(t, string(componentURN), file.NameTable["component"])
	assert.Equal(t, string(providerURN), file.NameTable["default"])
	require.Len(t, file.Resources, 1)
	res := file.Resources[0]
	assert.Equal(t, "aws:s3/bucket:Bucket", res.Type)
	assert.Equal(t, "bucket", res.Name)
	assert.Equal(t, "bucket-123", res.ID)
	assert.Equal(t, "MyBucket", res.LogicalName)
	assert.Equal(t, []string{"tags"}, res.Properties)
	assert.False(t, res.Component)
	assert.Equal(t, "component", res.Parent)
	assert.Equal(t, "default", res.Provider)
	assert.Equal(t, "7.11.0", res.Version)
}

func TestBuildFileFromDeploymentWithoutStateFallsBackToCaptures(t *testing.T) {
	t.Parallel()

	captures := []CaptureMetadata{
		{Type: "aws:s3/bucket:Bucket", Name: "a", LogicalName: "LogicalA", ID: "one"},
		{Type: "aws:s3/bucket:Bucket", Name: "b", LogicalName: "LogicalB", ID: "two"},
	}

	file, err := BuildFileFromDeployment(apitype.UntypedDeployment{}, captures)
	require.NoError(t, err)
	assert.Nil(t, file.NameTable)
	require.Len(t, file.Resources, 2)
	assert.Equal(t, "a", file.Resources[0].Name)
	assert.Equal(t, "LogicalA", file.Resources[0].LogicalName)
	assert.Equal(t, "one", file.Resources[0].ID)
}
