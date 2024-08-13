package main

import (
	_ "embed"
	"encoding/json"

	"github.com/pulumi/pulumi-aws-native/provider/pkg/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// See [awsNativeMetadata] var to access this.
type awsNativeMetadataSource struct {
	cloudApiMetadata metadata.CloudAPIMetadata
}

// Convert a Pulumi resource token into the matching CF ResourceType.
func (src *awsNativeMetadataSource) ResourceType(resourceToken tokens.Type) (ResourceType, bool) {
	r, ok := src.cloudApiMetadata.Resources[string(resourceToken)]
	if !ok {
		return "", false
	}
	return ResourceType(r.CfType), true
}

// Inverse of [ResourceType].
func (src *awsNativeMetadataSource) ResourceToken(resourceType ResourceType) (tokens.Type, bool) {
	// TODO pre-compute the reverse map.
	for tok, r := range src.cloudApiMetadata.Resources {
		if r.CfType == string(resourceType) {
			return tokens.Type(tok), true
		}
	}
	return "", false
}

// Find which Pulumi properties are needed to construct a Primary Resource Identifier.
//
// See https://docs.aws.amazon.com/cloudcontrolapi/latest/userguide/resource-identifier.html
func (src *awsNativeMetadataSource) PrimaryIdentifier(resourceToken tokens.Type) ([]resource.PropertyKey, bool) {
	r, ok := src.cloudApiMetadata.Resources[string(resourceToken)]
	if !ok {
		return nil, false
	}
	props := []resource.PropertyKey{}
	for _, rawProp := range r.PrimaryIdentifier {
		prop := resource.PropertyKey(rawProp)
		props = append(props, prop)
	}
	return props, true
}

//go:embed pulumi-aws-native-metadata.json
var awsNativeMetadataBytes []byte

var awsNativeMetadata *awsNativeMetadataSource

func init() {
	var m metadata.CloudAPIMetadata
	if err := json.Unmarshal(awsNativeMetadataBytes, &m); err != nil {
		panic(err)
	}
	awsNativeMetadata = &awsNativeMetadataSource{m}
}
