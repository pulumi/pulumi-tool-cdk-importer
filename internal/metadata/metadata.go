package metadata

import (
	providerMetadata "github.com/pulumi/pulumi-aws-native/provider/pkg/metadata"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type MetadataSource interface {
	ResourceType(resourceToken tokens.Type) (common.ResourceType, bool)
	ResourceToken(resourceType common.ResourceType) (tokens.Type, bool)
	PrimaryIdentifier(resourceToken tokens.Type) ([]resource.PropertyKey, bool)
	Resource(resourceToken string) (providerMetadata.CloudAPIResource, error)
	Separator(resourceToken tokens.Type) string
}
