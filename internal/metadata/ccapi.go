// Copyright 2016-2024, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metadata

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-aws-native/provider/pkg/metadata"
	"github.com/pulumi/pulumi-aws-native/provider/pkg/naming"
	"github.com/pulumi/pulumi-go-provider/resourcex"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

func NewCCApiMetadataSource() *awsNativeMetadataSource {
	return awsNativeMetadata
}

// See [awsNativeMetadata] var to access this.
type awsNativeMetadataSource struct {
	cloudApiMetadata metadata.CloudAPIMetadata
}

// Convert a Pulumi resource token into the matching CF ResourceType.
func (src *awsNativeMetadataSource) ResourceType(resourceToken tokens.Type) (common.ResourceType, bool) {
	r, ok := src.cloudApiMetadata.Resources[string(resourceToken)]
	if !ok {
		return "", false
	}
	return common.ResourceType(r.CfType), true
}

// Inverse of [ResourceType].
func (src *awsNativeMetadataSource) ResourceToken(resourceType common.ResourceType) (tokens.Type, bool) {
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

func (src *awsNativeMetadataSource) Resource(resourceToken string) (metadata.CloudAPIResource, error) {
	r, ok := src.cloudApiMetadata.Resources[resourceToken]
	if !ok {
		return metadata.CloudAPIResource{}, fmt.Errorf("Could not find resource: %s", resourceToken)
	}
	return r, nil
}

func (src *awsNativeMetadataSource) CfnProperties(resourceToken string, inputs resource.PropertyMap) (map[string]any, error) {
	inputsMap := resourcex.Decode(inputs)
	spec, err := src.Resource(resourceToken)
	if err != nil {
		return nil, err
	}

	// Convert SDK inputs to CFN payload.
	payload, err := naming.SdkToCfn(&spec, src.cloudApiMetadata.Types, inputsMap)
	if err != nil {
		return nil, fmt.Errorf("Failed to convert value for %s: %w", resourceToken, err)
	}
	return payload, nil

}

//go:embed schemas/pulumi-aws-native-metadata.json
var awsNativeMetadataBytes []byte

var awsNativeMetadata *awsNativeMetadataSource

func init() {
	var m metadata.CloudAPIMetadata
	if err := json.Unmarshal(awsNativeMetadataBytes, &m); err != nil {
		panic(err)
	}
	awsNativeMetadata = &awsNativeMetadataSource{m}
}
