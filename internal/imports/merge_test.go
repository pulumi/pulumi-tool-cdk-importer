package imports

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeWithSkeletonPrefersEnrichedData(t *testing.T) {
	t.Parallel()

	skeleton := &File{
		NameTable: map[string]string{"a": "urn:a"},
		Resources: []Resource{{
			Type:        "aws:s3/bucket:Bucket",
			Name:        "bucket",
			LogicalName: "Bucket",
			ID:          placeholderID,
			Version:     "1.0.0",
		}},
	}
	enriched := &File{
		NameTable: map[string]string{"b": "urn:b"},
		Resources: []Resource{{
			Type:        "aws:s3/bucket:Bucket",
			Name:        "bucket",
			LogicalName: "MyBucket",
			ID:          "real-id",
			Provider:    "default",
			Parent:      "parent",
			Version:     "2.0.0",
			Properties:  []string{"tags"},
		}},
	}

	merged := MergeWithSkeleton(skeleton, enriched)
	require.NotNil(t, merged)
	assert.Equal(t, map[string]string{"a": "urn:a", "b": "urn:b"}, merged.NameTable)
	require.Len(t, merged.Resources, 1)
	res := merged.Resources[0]
	assert.Equal(t, "aws:s3/bucket:Bucket", res.Type)
	assert.Equal(t, "bucket", res.Name)
	assert.Equal(t, "Bucket", res.LogicalName)
	assert.Equal(t, "real-id", res.ID)
	assert.Equal(t, "default", res.Provider)
	assert.Equal(t, "parent", res.Parent)
	assert.Equal(t, "1.0.0", res.Version)
	assert.Equal(t, []string{"tags"}, res.Properties)
}

func TestMergeWithSkeletonKeepsSkeletonOnlyEntries(t *testing.T) {
	t.Parallel()

	skeleton := &File{
		Resources: []Resource{{
			Type:        "aws:sqs/queue:Queue",
			Name:        "queue",
			LogicalName: "Queue",
			ID:          placeholderID,
		}},
	}
	enriched := &File{}

	merged := MergeWithSkeleton(skeleton, enriched)
	require.Len(t, merged.Resources, 1)
	assert.Equal(t, placeholderID, merged.Resources[0].ID)
}
