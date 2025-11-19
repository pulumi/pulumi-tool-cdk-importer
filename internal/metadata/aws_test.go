package metadata

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
)

func TestAwsClassicMetadataSourceSeparator(t *testing.T) {
	src := NewAwsMetadataSource()

	t.Run("returns colon for RolePolicy", func(t *testing.T) {
		sep := src.Separator(tokens.Type("aws:iam/rolePolicy:RolePolicy"))
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
