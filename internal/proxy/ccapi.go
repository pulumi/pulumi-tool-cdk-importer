package proxy

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

type awsCCApiInterceptor struct {
	client lookups.Lookups
}

func (i *awsCCApiInterceptor) create(
	ctx context.Context,
	in *pulumirpc.CreateRequest,
	client pulumirpc.ResourceProviderClient,
) (*pulumirpc.CreateResponse, error) {
	c := i.client
	urn, err := resource.ParseURN(in.GetUrn())
	if err != nil {
		return nil, err
	}
	label := fmt.Sprintf("%s.Create(%s)", "aws-native-proxy", urn)
	resourceToken := string(urn.Type())

	inputs, err := plugin.UnmarshalProperties(in.GetProperties(), plugin.MarshalOptions{
		Label:        fmt.Sprintf("%s.properties", label),
		KeepUnknowns: true,
		RejectAssets: true,
		KeepSecrets:  true,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "malformed resource inputs")
	}

	awsNativeMetadata := metadata.NewCCApiMetadataSource()
	props, err := awsNativeMetadata.CfnProperties(resourceToken, inputs)
	if err != nil {
		return nil, err
	}

	// find the corresponding CloudFormation resource
	logical, err := c.FindLogicalResourceID(urn)
	if err != nil {
		return nil, err
	}
	prim, err := c.FindPrimaryResourceID(ctx, urn.Type(), logical, props)
	if err != nil {
		return nil, err
	}
	rresp, err := client.Read(ctx, &pulumirpc.ReadRequest{
		Id:  string(prim),
		Urn: string(urn),
	})
	if err != nil {
		return nil, fmt.Errorf("Import failed: %w", err)
	}
	return &pulumirpc.CreateResponse{
		Id:         rresp.Id,
		Properties: rresp.Properties,
	}, nil
}
