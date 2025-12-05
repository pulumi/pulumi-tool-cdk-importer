package imports

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

const placeholderID = "<PLACEHOLDER>"

// File models the structure expected by `pulumi import --file`.
type File struct {
	// A mapping from in-language variable names to URNs. Used when generating references to parents and providers
	NameTable map[string]string `json:"nameTable"`
	// The list of resources to import
	Resources []Resource `json:"resources"`
}

// Resource represents a single resource import entry.
type Resource struct {
	// The type of the corresponding Pulumi resource
	Type string `json:"type"`
	// The name of the resource
	Name string `json:"name"`
	// The provider determined ID for this resource type. This is required unless `Component` is `true`
	ID string `json:"id,omitempty"`
	// The logical name of the resource. The original `Name` property is then used just for codegen
	// purposes (i.e. the source name). If either property is not set, then the other field is used to fill it in
	LogicalName string `json:"logicalName,omitempty"`
	// The list of properties to include in the generated code. If unspecified all properties will be included
	Properties []string `json:"properties,omitempty"`
	// This import should create an empty component resource. `id` must not be set if this is `true`
	Component bool `json:"component,omitempty"`
	// The version of the provider to use
	Version string `json:"version,omitempty"`
	// The name of the parent resource. The mentioned name must be present in the `NameTable`
	Parent string `json:"parent,omitempty"`
	// The name of the provider resource. The mentioned name must be present in the `NameTable`
	Provider string `json:"provider,omitempty"`
}

// Summary captures high-level details about what was emitted.
type Summary struct {
	TotalResources     int
	EmittedResources   int
	SkippedResources   []SkippedResource
	PlaceholderEntries []PlaceholderEntry
}

// SkippedResource records resources we did not include in the output.
type SkippedResource struct {
	LogicalID    common.LogicalResourceID
	ResourceType common.ResourceType
	Reason       string
}

// PlaceholderEntry captures resources where we could not determine an ID.
type PlaceholderEntry struct {
	LogicalID    common.LogicalResourceID
	ResourceType common.ResourceType
	Error        string
}

// BuildImportFile enumerates the CloudFormation stack resources tracked by the
// provided lookups instance and returns an import file plus a summary.
func BuildImportFile(ctx context.Context, l *lookups.Lookups) (*File, *Summary, error) {
	summary := &Summary{
		TotalResources: len(l.CfnStackResources),
	}

	keys := make([]string, 0, len(l.CfnStackResources))
	for logical := range l.CfnStackResources {
		keys = append(keys, string(logical))
	}
	sort.Strings(keys)

	resourceEntries := make([]Resource, 0, len(keys))
	metadataSrc := metadata.NewAwsMetadataSource()

	for _, logicalKey := range keys {
		logicalID := common.LogicalResourceID(logicalKey)
		stackResource := l.CfnStackResources[logicalID]
		if skip, reason := shouldSkipResource(stackResource.ResourceType); skip {
			summary.SkippedResources = append(summary.SkippedResources, SkippedResource{
				LogicalID:    logicalID,
				ResourceType: stackResource.ResourceType,
				Reason:       reason,
			})
			continue
		}

		token, ok := metadataSrc.ResourceToken(stackResource.ResourceType)
		if !ok {
			token, ok = defaultClassicTokenFromCFType(stackResource.ResourceType)
		}
		if !ok {
			summary.SkippedResources = append(summary.SkippedResources, SkippedResource{
				LogicalID:    logicalID,
				ResourceType: stackResource.ResourceType,
				Reason:       "unsupported resource type",
			})
			continue
		}

		id := string(stackResource.PhysicalID)
		if id == "" {
			summary.PlaceholderEntries = append(summary.PlaceholderEntries, PlaceholderEntry{
				LogicalID:    logicalID,
				ResourceType: stackResource.ResourceType,
				Error:        "missing physical ID",
			})
			id = placeholderID
		}

		resourceEntries = append(resourceEntries, Resource{
			Type:        string(token),
			Name:        resourceName(logicalID),
			ID:          id,
			LogicalName: string(logicalID),
		})
	}

	sort.Slice(resourceEntries, func(i, j int) bool {
		return resourceEntries[i].Name < resourceEntries[j].Name
	})

	summary.EmittedResources = len(resourceEntries)

	return &File{
		Resources: resourceEntries,
	}, summary, nil
}

// WriteFile marshals the File as prettified JSON.
func WriteFile(path string, file *File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

// ReadFile unmarshals an import file from disk.
func ReadFile(path string) (*File, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file File
	if err := json.Unmarshal(bytes, &file); err != nil {
		return nil, err
	}
	return &file, nil
}

func resourceName(logicalID common.LogicalResourceID) string {
	name := string(logicalID)
	if name == "" {
		return "resource"
	}
	return name
}

func shouldSkipResource(resourceType common.ResourceType) (bool, string) {
	switch resourceType {
	case "":
		return true, "missing resource type"
	case "AWS::CDK::Metadata":
		return true, "CDK metadata"
	case "AWS::CloudFormation::Stack":
		return true, "nested CloudFormation stack"
	}
	if strings.HasPrefix(string(resourceType), "Custom::") {
		return true, "custom resource"
	}
	return false, ""
}

func defaultClassicTokenFromCFType(resourceType common.ResourceType) (tokens.Type, bool) {
	parts := strings.Split(string(resourceType), "::")
	if len(parts) != 3 || parts[0] != "AWS" {
		return "", false
	}
	service := strings.ToLower(parts[1])
	resourceName := parts[2]
	if resourceName == "" {
		return "", false
	}
	module := strings.ToLower(resourceName[:1]) + resourceName[1:]
	token := fmt.Sprintf("aws:%s/%s:%s", service, module, resourceName)
	return tokens.Type(token), true
}

// FilterPlaceholderResources returns a copy of the given file that only contains resources whose IDs
// are still unresolved placeholders. NameTable is preserved as-is to keep parent/provider references intact.
func FilterPlaceholderResources(file *File) *File {
	if file == nil {
		return nil
	}

	filtered := make([]Resource, 0, len(file.Resources))
	for _, res := range file.Resources {
		if strings.EqualFold(res.ID, placeholderID) {
			filtered = append(filtered, Resource{
				Type:        res.Type,
				Name:        res.Name,
				ID:          res.ID,
				LogicalName: res.LogicalName,
				Properties:  cloneStrings(res.Properties),
				Component:   res.Component,
				Version:     res.Version,
				Parent:      res.Parent,
				Provider:    res.Provider,
			})
		}
	}

	return &File{
		NameTable: file.NameTable,
		Resources: filtered,
	}
}
