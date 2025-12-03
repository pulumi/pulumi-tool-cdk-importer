package metadata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

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

type primaryIdentifierSet struct {
	Format string   `json:"format"`
	Parts  []string `json:"parts"`
}

type primaryIdentifierEntry struct {
	Note              string               `json:"note,omitempty"`
	Provider          string               `json:"provider"`
	PrimaryIdentifier primaryIdentifierSet `json:"primaryIdentifier"`
	PulumiTypes       []string             `json:"pulumiTypes"`
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

// deriveSeparator attempts to infer the separator between identifier parts from the provided format string.
func deriveSeparator(format string, parts []string) string {
	if len(parts) < 2 || format == "" {
		return "/"
	}

	remaining := format
	if idx := strings.Index(remaining, parts[0]); idx >= 0 {
		remaining = remaining[idx+len(parts[0]):]
	}

	nextIdx := strings.Index(remaining, parts[1])
	if nextIdx < 0 {
		return "/"
	}

	sep := remaining[:nextIdx]
	if sep == "" {
		return "/"
	}
	return sep
}

var awsClassicMetadata *awsClassicMetadataSource

//go:embed schemas/primary-identifiers.json
var primaryIdentifiersBytes []byte

func init() {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(primaryIdentifiersBytes, &raw); err != nil {
		panic(fmt.Errorf("decoding primary identifiers: %w", err))
	}

	type resourceCandidate struct {
		resource metadata.CloudAPIResource
		count    int
		sep      string
	}

	candidates := map[string]resourceCandidate{}

	for cfType, rawEntry := range raw {
		var entries []primaryIdentifierEntry
		if len(rawEntry) > 0 && rawEntry[0] == '[' {
			if err := json.Unmarshal(rawEntry, &entries); err != nil {
				panic(fmt.Errorf("decoding primary identifier array for %s: %w", cfType, err))
			}
		} else {
			var entry primaryIdentifierEntry
			if err := json.Unmarshal(rawEntry, &entry); err != nil {
				panic(fmt.Errorf("decoding primary identifier for %s: %w", cfType, err))
			}
			entries = append(entries, entry)
		}

		for _, entry := range entries {
			if entry.Provider != "aws" {
				continue
			}

			count := len(entry.PulumiTypes)
			for _, pulumiType := range entry.PulumiTypes {
				sep := deriveSeparator(entry.PrimaryIdentifier.Format, entry.PrimaryIdentifier.Parts)
				// Prefer more specific mappings (fewer pulumiTypes attached to a CF type).
				if existing, ok := candidates[pulumiType]; ok && existing.count <= count {
					continue
				}
				candidates[pulumiType] = resourceCandidate{
					resource: metadata.CloudAPIResource{
						CfType:            cfType,
						PrimaryIdentifier: entry.PrimaryIdentifier.Parts,
					},
					count: count,
					sep:   sep,
				}
			}
		}
	}

	// Manual overrides for resources that are either absent from the schema or require custom identifiers.
	manualResources := map[string]metadata.CloudAPIResource{
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
		"aws:servicediscovery/privateDnsNamespace:PrivateDnsNamespace": {
			CfType: "AWS::ServiceDiscovery::PrivateDnsNamespace",
			PrimaryIdentifier: []string{
				"id",
				"vpc",
			},
		},
	}
	for tok, res := range manualResources {
		candidates[tok] = resourceCandidate{
			resource: res,
			count:    0,
			sep:      deriveSeparator("", res.PrimaryIdentifier),
		}
	}

	manualSeparators := map[string]string{
		"aws:iam/rolePolicy:RolePolicy":                                ":",
		"aws:servicediscovery/privateDnsNamespace:PrivateDnsNamespace": ":",
	}
	resources := map[string]metadata.CloudAPIResource{}
	separators := map[string]string{}
	for tok, candidate := range candidates {
		resources[tok] = candidate.resource
		sep := candidate.sep
		if override, ok := manualSeparators[tok]; ok {
			sep = override
		}
		if sep != "/" {
			separators[tok] = sep
		}
	}

	awsClassicMetadata = &awsClassicMetadataSource{
		separator: separators,
		cloudApiMetadata: metadata.CloudAPIMetadata{
			Resources: resources,
		},
	}

}
