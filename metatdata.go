package main

import (
	"github.com/pulumi/pulumi-aws-native/provider/pkg/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type MetadataSource interface {
	ResourceType(resourceToken tokens.Type) (ResourceType, bool)
	ResourceToken(resourceType ResourceType) (tokens.Type, bool)
	PrimaryIdentifier(resourceToken tokens.Type) ([]resource.PropertyKey, bool)
	Resource(resourceToken string) (metadata.CloudAPIResource, error)
}
