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
	Resources []Resource `json:"resources"`
}

// Resource represents a single resource import entry.
type Resource struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	ID          string `json:"id,omitempty"`
	LogicalName string `json:"logicalName,omitempty"`
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
