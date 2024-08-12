package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

type identifierType string

const (
	identifierType_INPUT  identifierType = "INPUT"
	identifierType_OUTPUT identifierType = "OUTPUT"
)

type IdentifierInfo struct {
	Name           string         `json:"name"`
	IdentifierType identifierType `json:"identifierType"`
}

type schemaInfo []IdentifierInfo

type CloudFormationSchemas interface {
}

type cloudformationSchemas struct {
	resourceTypes map[string]schemaInfo
}

func newCloudformationSchemas() *cloudformationSchemas {
	return &cloudformationSchemas{
		resourceTypes: map[string]schemaInfo{},
	}
}

func (c *cloudformationSchemas) getResourceType(ctx context.Context, cfnType string) (schemaInfo, error) {
	if val, ok := c.resourceTypes[cfnType]; ok {
		return val, nil
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	client := cloudformation.NewFromConfig(cfg)
	out, err := client.DescribeType(ctx, &cloudformation.DescribeTypeInput{
		TypeName: &cfnType,
		Type:     types.RegistryTypeResource,
	})
	if err != nil {
		return nil, err
	}

	if out.Schema == nil {
		return nil, fmt.Errorf("Schema for type %s is empty", cfnType)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(*out.Schema), &schema); err != nil {
		return nil, err
	}

	_, infos, err := processSchemaJson(schema)
	if err != nil {
		return nil, err
	}
	c.resourceTypes[cfnType] = infos
	return infos, nil
}

func processSchemaJson(schema map[string]interface{}) (string, schemaInfo, error) {
	typeName := schema["typeName"].(string)
	var createOnlyProperties []interface{}
	var readOnlyProperties []interface{}
	primaryIds, ok := schema["primaryIdentifier"].([]interface{})
	if !ok {
		fmt.Println(schema)
		return "", nil, fmt.Errorf("%s: primaryIdentifier doesn't exist", typeName)
	}
	cp, ok := schema["createOnlyProperties"]
	if ok {
		createOnlyProperties = cp.([]interface{})
	}
	rp, ok := schema["readOnlyProperties"]
	if ok {
		readOnlyProperties = rp.([]interface{})

	}

	infos := schemaInfo{}
	var idType identifierType
	for _, name := range primaryIds {
		stringName := name.(string)
		if slices.Contains(createOnlyProperties, name) {
			idType = identifierType_INPUT
		} else if slices.Contains(readOnlyProperties, name) {
			idType = identifierType_OUTPUT
		} else {
			// otherwise it's both so treat it as an input
			idType = identifierType_INPUT
		}
		infos = append(infos, IdentifierInfo{
			Name:           stringName,
			IdentifierType: idType,
		})
	}
	return typeName, infos, nil

}
