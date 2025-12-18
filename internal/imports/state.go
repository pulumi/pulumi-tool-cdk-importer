package imports

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// CaptureMetadata represents supplemental data collected during provider interception.
type CaptureMetadata struct {
	Type        string
	Name        string
	LogicalName string
	ID          string
	Properties  []string
}

// BuildFileFromDeployment merges Pulumi state with capture metadata to construct an import file.
func BuildFileFromDeployment(deployment apitype.UntypedDeployment, captures []CaptureMetadata) (*File, error) {
	typed, err := unmarshalDeployment(deployment)
	if err != nil {
		return nil, err
	}
	if typed == nil {
		return buildFileFromCaptures(captures), nil
	}
	return mergeStateAndCaptures(typed, captures)
}

func unmarshalDeployment(deployment apitype.UntypedDeployment) (*apitype.DeploymentV3, error) {
	if len(deployment.Deployment) == 0 {
		return nil, nil
	}
	var typed apitype.DeploymentV3
	if err := json.Unmarshal(deployment.Deployment, &typed); err != nil {
		return nil, fmt.Errorf("decoding exported deployment: %w", err)
	}
	return &typed, nil
}

func buildFileFromCaptures(captures []CaptureMetadata) *File {
	resources := make([]Resource, 0, len(captures))
	for _, capture := range captures {
		resources = append(resources, Resource{
			Type:        capture.Type,
			Name:        capture.Name,
			ID:          capture.ID,
			LogicalName: capture.LogicalName,
			Properties:  cloneStrings(capture.Properties),
		})
	}
	sortResources(resources)
	return &File{Resources: resources}
}

func mergeStateAndCaptures(deployment *apitype.DeploymentV3, captures []CaptureMetadata) (*File, error) {
	captureIndex := indexCaptures(captures)
	providers := collectProviderDetails(deployment.Resources)

	resources := make([]Resource, 0, len(deployment.Resources))
	for _, res := range deployment.Resources {
		if !isAWSResource(res.Type) {
			continue
		}

		name := res.URN.Name()
		if name == "" {
			name = "resource"
		}

		_, version, err := resolveProvider(res.Provider, providers)
		if err != nil {
			return nil, err
		}

		key := captureKey(string(res.Type), name)
		capture := captureIndex[key]

		logical := name
		id := capture.ID
		props := []string(nil)

		if capture.LogicalName != "" {
			logical = capture.LogicalName
		}
		if id == "" || id == placeholderID {
			id = string(res.ID)
		}
		if id == "" {
			id = placeholderID
		}
		if len(capture.Properties) > 0 {
			props = cloneStrings(capture.Properties)
		}

		resources = append(resources, Resource{
			Type:        string(res.Type),
			Name:        name,
			ID:          id,
			LogicalName: logical,
			Properties:  props,
			Component:   !res.Custom,
			Version:     version,
		})
	}

	sortResources(resources)

	return &File{
		Resources: resources,
	}, nil
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	dup := make([]string, len(values))
	copy(dup, values)
	return dup
}

func sortResources(resources []Resource) {
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Type == resources[j].Type {
			return resources[i].Name < resources[j].Name
		}
		return resources[i].Type < resources[j].Type
	})
}

func buildNameTable(resources []apitype.ResourceV3) map[string]string {
	table := make(map[string]string, len(resources))
	for _, res := range resources {
		name := res.URN.Name()
		if name == "" {
			continue
		}
		if _, exists := table[name]; exists {
			continue
		}
		table[name] = string(res.URN)
	}
	return table
}

func indexCaptures(captures []CaptureMetadata) map[string]CaptureMetadata {
	idx := make(map[string]CaptureMetadata, len(captures))
	for _, capture := range captures {
		key := captureKey(capture.Type, capture.Name)
		if key == "" {
			continue
		}
		idx[key] = capture
	}
	return idx
}

func captureKey(typ, name string) string {
	if typ == "" || name == "" {
		return ""
	}
	return typ + "|" + name
}

func collectProviderDetails(resources []apitype.ResourceV3) map[string]providerDetails {
	providers := make(map[string]providerDetails)
	for _, res := range resources {
		if !strings.HasPrefix(string(res.Type), "pulumi:providers:") {
			continue
		}
		version := readString(res.Inputs, "version")
		if version == "" {
			version = readString(res.Outputs, "version")
		}
		providers[string(res.URN)] = providerDetails{
			name:    res.URN.Name(),
			version: version,
		}
	}
	return providers
}

type providerDetails struct {
	name    string
	version string
}

func readString(props map[string]any, key string) string {
	if len(props) == 0 {
		return ""
	}
	if value, ok := props[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func resolveParentName(parent resource.URN) (string, error) {
	if parent == "" {
		return "", nil
	}
	if !parent.IsValid() {
		return "", fmt.Errorf("invalid parent URN %q", parent)
	}
	if parent.QualifiedType() == resource.RootStackType {
		return "", nil
	}
	return parent.Name(), nil
}

func resolveProvider(provider string, providers map[string]providerDetails) (string, string, error) {
	if provider == "" {
		return "", "", nil
	}
	if details, ok := providers[provider]; ok {
		return details.name, details.version, nil
	}
	urn, err := resource.ParseURN(provider)
	if err != nil {
		return "", "", fmt.Errorf("invalid provider URN %q: %w", provider, err)
	}
	return urn.Name(), "", nil
}

func isAWSResource(typ tokens.Type) bool {
	str := string(typ)
	return strings.HasPrefix(str, "aws:") || strings.HasPrefix(str, "aws-native:")
}
