package proxy

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

type awsInterceptor struct {
	*lookups.Lookups
}

func (i *awsInterceptor) create(
	ctx context.Context,
	in *pulumirpc.CreateRequest,
	client pulumirpc.ResourceProviderClient,
) (*pulumirpc.CreateResponse, error) {
	c := lookups.NewAwsLookups(i.CfnStackResources, i.Region, i.Account)
	urn, err := resource.ParseURN(in.GetUrn())
	if err != nil {
		return nil, err
	}
	resourceType := string(urn.Type())

	// These resources are only mapped to classic resources in the synthesizer so these
	// resources won't be imported
	switch resourceType {
	case "aws:s3/bucketObjectv2:BucketObjectv2":
		fallthrough
	case "aws:s3/bucketV2:BucketV2":
		fallthrough
	case "aws:s3/bucketLifecycleConfigurationV2:BucketLifecycleConfigurationV2":
		fallthrough
	case "aws:s3/bucketServerSideEncryptionConfigurationV2:BucketServerSideEncryptionConfigurationV2":
		fallthrough
	case "aws:s3/bucketPolicy:BucketPolicy":
		fallthrough
	case "aws:s3/bucketVersioningV2:BucketVersioningV2":
		fallthrough
	case "aws:ecr/repository:Repository":
		fallthrough
	case "aws:ecr/lifecyclePolicy:LifecyclePolicy":
		fallthrough
	// These resources are incorrectly mapped. In CDK they are inline policies,
	// but they are mapped to actual policy resources so they can't be imported.
	// Just create them as new resources
	case "aws:iam/policy:Policy":
		fallthrough
	case "aws:iam/rolePolicyAttachment:RolePolicyAttachment":
		glog.V(1).Infof("Resource type %s is not supported for import, creating instead", resourceType)
		return client.Create(ctx, in)
	}
	label := fmt.Sprintf("%s.Create(%s)", "aws-proxy", urn)
	inputs, err := plugin.UnmarshalProperties(in.GetProperties(), plugin.MarshalOptions{
		Label:        fmt.Sprintf("%s.properties", label),
		KeepUnknowns: true,
		RejectAssets: true,
		KeepSecrets:  true,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "malformed resource inputs")
	}
	logical, err := c.FindLogicalResourceID(urn)
	if err != nil {
		return nil, err
	}
	prim, err := c.FindPrimaryResourceID(ctx, urn.Type(), logical, inputs.Mappable())
	if err != nil {
		return nil, err
	}

	glog.V(1).Infof("Importing resourceType %s with ID %s for URN %s ...", resourceType, string(prim), string(urn))
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
