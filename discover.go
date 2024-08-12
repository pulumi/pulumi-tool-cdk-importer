package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
)

type CFResourceType string       // ex. AWS::S3::Bucket
type CFPrimaryResourceID string  // ex. "${DatabaseName}|${TableName}"
type CDKLogicalResourceID string // ex. t0yv0Bucket1EAC1B2B

func NewCloudControlClient(ctx context.Context) (*cloudcontrol.Client, error) {
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return cloudcontrol.NewFromConfig(awsConfig), nil
}

type CFTag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

type CFTagsResourceModel struct {
	Tags []CFTag `json:"Tags"`
}

func DiscoverByCDKLogicalResourceID(
	ctx context.Context,
	client *cloudcontrol.Client,
	ty CFResourceType,
	id CDKLogicalResourceID,
) ([]CFPrimaryResourceID, error) {
	searchFor := CFTagsResourceModel{
		//Bucket: "t0yv0-cdk-test-app-dev-t0yv0bucket1eac1b2b-bl5nfw14gkpj",
		Tags: []CFTag{
			{
				Key:   "aws:cloudformation:logical-id",
				Value: string(id),
			},
		},
	}

	searchForBytes, err := json.Marshal(searchFor)
	if err != nil {
		return nil, err
	}

	result, err := client.ListResources(ctx, &cloudcontrol.ListResourcesInput{
		TypeName:      aws.String(string(ty)),
		ResourceModel: aws.String(string(searchForBytes)),
	})
	if err != nil {
		return nil, err
	}
	// TODO follow tokens if result is paginated? or assert there's none.

	found := []CFPrimaryResourceID{}

	for _, res := range result.ResourceDescriptions {
		resid := res.Identifier
		if resid == nil {
			return nil, fmt.Errorf("Unexpected cloudcontrol.Client.ListResources result: ResourceDescription.Identifier is nil")
		}
		found = append(found, CFPrimaryResourceID(*resid))
	}

	return found, nil
}

// func GenerateImportFile(manifest CDKManifest) {
//    for _, knownResource := range manifest.KnownResources() {
//      ids := Discover(knownResource.Type, knownResource.LogicalResourceID)
//      if len(ids) != ! {
//        panic("Confused..")
//      }
//      id := ids[0]
//      importEntry := importEntry{
//        Type: knownResource.Type,
//        Id:   id,
//        Name: "???" // TODO What should be the Pulumi name to match the code?
//      }
//      entries := append(entries, importEntry)
//    }
// }

//results := ccapi.ListResources( /* search tags: aws:cloudformation:logical_id=$id */ )
// search for Primary Identifier in Results
// So this should return the primary identifier as a result of ListResources:
// https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/cloudcontrol@v1.20.3/types#ResourceDescription
