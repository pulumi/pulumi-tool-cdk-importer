package proxy

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	nResources "github.com/pulumi/pulumi-aws-native/provider/pkg/resources"
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

	if resourceToken == "aws-native:cloudformation:CustomResourceEmulator" {
		return nil, errors.New("CustomResourceEmulator is not supported")
	}

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
	glog.V(1).Infof("Importing resourceType %s with ID %s for URN %s ...", urn.Type().String(), string(prim), string(urn))
	rresp, err := client.Read(ctx, &pulumirpc.ReadRequest{
		Id:  string(prim),
		Urn: string(urn),
	})
	if err != nil {
		return nil, fmt.Errorf("Import failed: %w", err)
	}

	spec, err := awsNativeMetadata.Resource(resourceToken)
	if err != nil {
		return nil, err
	}
	outputs, err := plugin.UnmarshalProperties(rresp.Properties, plugin.MarshalOptions{
		Label:        fmt.Sprintf("%s.outputs", label),
		KeepUnknowns: true,
		RejectAssets: true,
		KeepSecrets:  true,
	})
	rawOutputs := outputs.Mappable()
	// Write-only properties are not returned in the outputs, so we assume they should have the same value we sent from the inputs.
	if len(spec.WriteOnly) > 0 {
		inputsMap := inputs.Mappable()
		for _, writeOnlyProp := range spec.WriteOnly {
			if _, ok := rawOutputs[writeOnlyProp]; !ok {
				inputValue, ok := inputsMap[writeOnlyProp]
				if ok {
					rawOutputs[writeOnlyProp] = inputValue
				}
			}
		}
	}
	outputs = nResources.CheckpointObject(inputs, rawOutputs)
	checkpoint, err := plugin.MarshalProperties(outputs, plugin.MarshalOptions{
		Label:        fmt.Sprintf("%s.outputs", label),
		KeepUnknowns: true,
		KeepSecrets:  true,
		RejectAssets: true,
	})

	return &pulumirpc.CreateResponse{
		Id:         rresp.Id,
		Properties: checkpoint,
	}, nil
}
