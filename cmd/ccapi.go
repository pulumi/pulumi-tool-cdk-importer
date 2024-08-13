package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

type CdkImportFile struct {
	Resources []Resource `json:"resources"`
}

type PulumiImportFile struct {
	Resources []PulumiResource `json:"resources"`
}

type PulumiResource struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	ID     string `json:"id"`
	Parent string `json:"parent"`
}

type Resource struct {
	Type     string       `json:"type"`
	Name     string       `json:"name"`
	ID       string       `json:"id"`
	Parent   string       `json:"parent"`
	LookupID []PropertyID `json:"lookupId,omitempty"`
}

type PropertyID struct {
	IdPropertyName  string `json:"idPropertyName"`
	LogicalID       string `json:"logicalId"`
	PropertyRefName string `json:"propertyRefName"`
}

func newCcapi(ctx context.Context) (*ccapi, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	client := cloudcontrol.NewFromConfig(cfg)
	cfnClient := cloudformation.NewFromConfig(cfg)
	return &ccapi{
		ccapiClient:        client,
		cfnClient:          cfnClient,
		cfnStackResources:  make(map[string]string),
		ccapiResourceCache: make(map[string][]types.ResourceDescription),
		processed:          make(map[string]Resource),
	}, nil
}

type ccapi struct {
	ccapiClient *cloudcontrol.Client
	cfnClient   *cloudformation.Client
	// map of logicalResourceId to physicalResourceId
	cfnStackResources  map[string]string
	ccapiResourceCache map[string][]types.ResourceDescription
	processed          map[string]Resource
}

func (c *ccapi) generateImportFile(ctx context.Context, location string, outFile string) error {
	contents, err := os.ReadFile(location)
	if err != nil {
		return err
	}

	if err := c.getStackResources(ctx, "chall-cdk-test-app-dev"); err != nil {
		return err
	}

	var importFile CdkImportFile
	if err := json.Unmarshal(contents, &importFile); err != nil {
		return err
	}

	pulumiFile := PulumiImportFile{
		Resources: []PulumiResource{},
	}
	deferred := map[string]Resource{}
	// Rough logic
	// - If resource.LookupID has a single entry then it has to be a reference to itself
	//   - If the reference to itself is '*Name' OR '*Id' then it is probably the physicalResourceId
	//   - Otherwise the reference to itself will be to '*Arn'
	//     - The arn will end in the physicalResourceId
	for _, resource := range importFile.Resources {
		if resource.LookupID != nil {
			// if there is only one lookupid then it should be referencing itself, which means
			// there are no references to other resources
			// TODO: validate above statement
			if len(resource.LookupID) == 1 {
				lookup := resource.LookupID[0]
				if lookup.LogicalID != resource.Name {
					panic("The logicalID doesn't match the id")
				}
				id, err := c.findOwnId(ctx, resource.Type, lookup)
				if err != nil {
					panic(err)
				}
				resource.ID = id
				// TODO: does the resource have enough information to use in the dependent resources?
				c.processed[resource.Name] = resource
			}

			if !allResolved(resource, c.processed) {
				deferred[resource.Name] = resource
				continue
			}
			id, err := c.processCompositeResource(ctx, resource)
			if err != nil {
				return err
			}
			resource.ID = id
			c.processed[resource.Name] = resource
		}

		for len(deferred) > 0 {
			for logicalId, resource := range deferred {
				if allResolved(resource, c.processed) {
					delete(deferred, logicalId)
					id, err := c.processCompositeResource(ctx, resource)
					if err != nil {
						return err
					}
					resource.ID = id
				}
			}
		}

		pulumiFile.Resources = append(pulumiFile.Resources, PulumiResource{
			Type:   resource.Type, // TODO: pulumi type
			ID:     resource.ID,
			Parent: resource.Parent,
			Name:   resource.Name,
		})
	}

	outContents, err := json.Marshal(pulumiFile)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outFile, outContents, 0666); err != nil {
		return err
	}

	return nil
}

func (c *ccapi) processCompositeResource(ctx context.Context, resource Resource) (string, error) {
	// all values are resolved
	var ownResource PropertyID
	resourceModel := map[string]string{}
	for _, lookup := range resource.LookupID {
		// the reference is to itself
		if resource.Name == lookup.LogicalID {
			ownResource = lookup
		} else {
			ref := c.processed[lookup.LogicalID]
			resourceModel[lookup.IdPropertyName] = ref.ID
		}
	}
	id, err := c.findCompositeId(ctx, resource.Type, ownResource, resourceModel)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (c *ccapi) findCompositeId(ctx context.Context, resourceType string, lookup PropertyID, resourceModel map[string]string) (string, error) {
	if physicalId, ok := c.cfnStackResources[lookup.LogicalID]; ok {
		id, err := c.findResourceIdentifierBySuffix(ctx, resourceType, physicalId, resourceModel)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return "", fmt.Errorf("Couldn't find id")
}

// findOwnId should only be used when the resource only has a single element in it's identifier
func (c *ccapi) findOwnId(ctx context.Context, resourceType string, lookup PropertyID) (string, error) {
	if strings.HasSuffix(lookup.IdPropertyName, "Name") || strings.HasSuffix(lookup.IdPropertyName, "Id") {
		if physicalId, ok := c.cfnStackResources[lookup.LogicalID]; ok {
			return physicalId, nil
		}
		return "", fmt.Errorf("Resource doesn't exist in this stack which isn't possible!")
	} else if strings.HasSuffix(lookup.IdPropertyName, "Arn") {
		if physicalId, ok := c.cfnStackResources[lookup.LogicalID]; ok {
			id, err := c.findResourceIdentifierBySuffix(ctx, resourceType, physicalId, nil)
			if err != nil {
				fmt.Errorf("Could not find id for %s: %w", lookup.LogicalID, err)
			}
			return id, nil
		}
	} else {
		return "", fmt.Errorf("Expected suffix of 'Id', 'Name', or 'Arn'; got %s", lookup.IdPropertyName)
	}
	return "", fmt.Errorf("Something happened")
}

func (c *ccapi) listResources(ctx context.Context, resourceType string, resourceModel map[string]string) ([]types.ResourceDescription, error) {
	if val, ok := c.ccapiResourceCache[resourceType]; ok {
		return val, nil
	}

	var model *string
	if len(resourceModel) > 0 {
		val, err := json.Marshal(resourceModel)
		if err != nil {
			return nil, err
		}
		stringVal := string(val)
		model = &stringVal
	}

	paginator := cloudcontrol.NewListResourcesPaginator(c.ccapiClient, &cloudcontrol.ListResourcesInput{
		ResourceModel: model,
		TypeName:      &resourceType,
	})
	resources := []types.ResourceDescription{}
	// TODO: we might be able to short circuit this if we find the correct one
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		resources = append(resources, output.ResourceDescriptions...)
	}

	c.ccapiResourceCache[resourceType] = resources
	return resources, nil
}

// This finds resources with Arn identifiers based on whether the Arn ends
// in the provided value
func (c *ccapi) findResourceIdentifierBySuffix(ctx context.Context, resourceType, suffix string, resourceModel map[string]string) (string, error) {
	resources, err := c.listResources(ctx, resourceType, resourceModel)
	if err != nil {
		var uae *types.UnsupportedActionException
		if errors.As(err, &uae) {
			fmt.Printf("ResourceType %q not yet supported by cloudcontrol, manual mapping required", resourceType)
		}
		return "<PLACEHOLDER>", nil
	}

	for _, resource := range resources {
		if strings.HasSuffix(*resource.Identifier, suffix) {
			return *resource.Identifier, nil
		}
	}

	return "", fmt.Errorf("could not find resource identifier for type: %s", resourceType)
}

func (c *ccapi) getStackResources(ctx context.Context, stackName string) error {
	paginator := cloudformation.NewListStackResourcesPaginator(c.cfnClient, &cloudformation.ListStackResourcesInput{
		StackName: &stackName,
	})

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, s := range output.StackResourceSummaries {
			c.cfnStackResources[*s.LogicalResourceId] = *s.PhysicalResourceId
		}
	}
	return nil
}

func allResolved(deferred Resource, processed map[string]Resource) bool {
	processedKeys := []string{}
	for key := range processed {
		processedKeys = append(processedKeys, key)
	}
	for _, lookup := range deferred.LookupID {
		if lookup.LogicalID != deferred.Name {
			if !slices.Contains(processedKeys, lookup.LogicalID) {
				return false
			}
		}
	}

	return true
}
