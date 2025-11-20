package metadata

import (
	"fmt"

	"github.com/pulumi/pulumi-aws-native/provider/pkg/metadata"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

func NewAwsMetadataSource() *awsClassicMetadataSource {
	return awsClassicMetadata
}

type awsClassicMetadataSource struct {
	cloudApiMetadata metadata.CloudAPIMetadata
	separator        map[string]string
}

// Convert a Pulumi resource token into the matching CF ResourceType.
func (src *awsClassicMetadataSource) ResourceType(resourceToken tokens.Type) (common.ResourceType, bool) {
	r, ok := src.cloudApiMetadata.Resources[string(resourceToken)]
	if !ok {
		return "", false
	}
	return common.ResourceType(r.CfType), true
}

// Inverse of [ResourceType].
func (src *awsClassicMetadataSource) ResourceToken(resourceType common.ResourceType) (tokens.Type, bool) {
	// TODO: pre-compute the reverse map.
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
func (src *awsClassicMetadataSource) PrimaryIdentifier(resourceToken tokens.Type) ([]resource.PropertyKey, bool) {
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

func (src *awsClassicMetadataSource) Resource(resourceToken string) (metadata.CloudAPIResource, error) {
	r, ok := src.cloudApiMetadata.Resources[resourceToken]
	if !ok {
		return metadata.CloudAPIResource{}, fmt.Errorf("Could not find resource: %s", resourceToken)
	}
	return r, nil
}

func (src *awsClassicMetadataSource) Separator(resourceToken tokens.Type) string {
	if sep, ok := src.separator[string(resourceToken)]; ok {
		return sep
	}
	return "/"
}

var awsClassicMetadata *awsClassicMetadataSource

func init() {
	awsClassicMetadata = &awsClassicMetadataSource{
		separator: map[string]string{
			"aws:iam/rolePolicy:RolePolicy":                                ":",
			"aws:servicediscovery/privateDnsNamespace:PrivateDnsNamespace": ":",
		},
		cloudApiMetadata: metadata.CloudAPIMetadata{
			Resources: map[string]metadata.CloudAPIResource{
				"aws:apigatewayv2/stage:Stage": {
					CfType: "AWS::ApiGatewayV2::Stage",
					PrimaryIdentifier: []string{
						"apiId",
						"name",
					},
				},
				"aws:apigatewayv2/integration:Integration": {
					CfType: "AWS::ApiGatewayV2::Integration",
					PrimaryIdentifier: []string{
						"apiId",
						"id",
					},
				},
				"aws:iam/policy:Policy": {
					CfType: "AWS::IAM::Policy",
					PrimaryIdentifier: []string{
						"arn",
					},
				},
				"aws:servicediscovery/service:Service": {
					CfType: "AWS::ServiceDiscovery::Service",
					PrimaryIdentifier: []string{
						"id",
					},
				},
				"aws:servicediscovery/privateDnsNamespace:PrivateDnsNamespace": {
					CfType: "AWS::ServiceDiscovery::PrivateDnsNamespace",
					PrimaryIdentifier: []string{
						"id",
						"vpc",
					},
				},
				"aws:iam/rolePolicy:RolePolicy": {
					CfType: "AWS::IAM::Policy",
					PrimaryIdentifier: []string{
						"role",
						"name",
					},
				},
				"aws:iam/rolePolicyAttachment:RolePolicyAttachment": {
					CfType: "AWS::IAM::Policy",
					PrimaryIdentifier: []string{
						"policyArn",
						"role",
					},
				},
			},
		},
	}

}
