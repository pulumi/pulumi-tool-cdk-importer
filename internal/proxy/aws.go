package proxy

import (
	"context"
	"fmt"
	"sort"

	"log/slog"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

type awsInterceptor struct {
	*lookups.Lookups
	mode       RunMode
	skipCreate bool
	collector  *CaptureCollector
	logger     *slog.Logger
}

func (i *awsInterceptor) create(
	ctx context.Context,
	in *pulumirpc.CreateRequest,
	client pulumirpc.ResourceProviderClient,
) (*pulumirpc.CreateResponse, error) {
	logger := i.logger
	if logger == nil {
		logger = slog.Default()
	}
	urn, err := resource.ParseURN(in.GetUrn())
	if err != nil {
		return nil, err
	}
	resourceType := string(urn.Type())

	// These resources are only mapped to classic resources in the synthesizer so these
	// resources won't be imported
	switch resourceType {
	case "aws:s3/bucketObjectv2:BucketObjectv2",
		"aws:s3/bucketV2:BucketV2",
		"aws:s3/bucketLifecycleConfigurationV2:BucketLifecycleConfigurationV2",
		"aws:s3/bucketServerSideEncryptionConfigurationV2:BucketServerSideEncryptionConfigurationV2",
		"aws:s3/bucketPolicy:BucketPolicy",
		"aws:s3/bucketVersioningV2:BucketVersioningV2",
		"aws:ecr/repository:Repository",
		"aws:ecr/lifecyclePolicy:LifecyclePolicy",
		"aws:iam/policy:Policy",
		"aws:iam/rolePolicyAttachment:RolePolicyAttachment":
		if i.skipCreate {
			return i.stubSkippedCreate(resourceType, urn, in)
		}
		if i.mode == CaptureImports {
			if i.collector != nil {
				i.collector.Skip(SkippedCapture{
					Type:        resourceType,
					LogicalName: string(urn.Name()),
					Reason:      "resource type not supported for capture",
				})
			}
			return nil, fmt.Errorf("resource type %s is not supported in capture mode", resourceType)
		}
		logger.Info("Resource type is not supported for import; creating instead", "resourceType", resourceType)
		return client.Create(ctx, in)
	}
	c := lookups.NewAwsLookups(i.CfnStackResources, i.Region, i.Account)
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

	logger.Debug("Importing resource", "resourceType", resourceType, "id", string(prim), "urn", string(urn))
	rresp, err := client.Read(ctx, &pulumirpc.ReadRequest{
		Id:  string(prim),
		Urn: string(urn),
	})
	if err != nil {
		return nil, fmt.Errorf("Import failed: %w", err)
	}
	if rresp.Id == "" {
		return nil, fmt.Errorf("Don't have an ID!: %s %s %s", resourceType, string(prim), string(urn))
	}

	if i.mode == CaptureImports && i.collector != nil {
		properties := collectPropertyKeys(inputs)
		i.collector.Append(Capture{
			Type:        resourceType,
			Name:        string(urn.Name()),
			LogicalName: string(logical),
			ID:          string(prim),
			Properties:  properties,
		})
	}

	return &pulumirpc.CreateResponse{
		Id:         rresp.Id,
		Properties: rresp.Properties,
	}, nil
}

func (i *awsInterceptor) stubSkippedCreate(resourceType string, urn resource.URN, req *pulumirpc.CreateRequest) (*pulumirpc.CreateResponse, error) {
	logger := i.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("Skipping creation due to skip-create flag", "logicalName", string(urn.Name()), "resourceType", resourceType)
	if i.collector != nil {
		i.collector.Skip(SkippedCapture{
			Type:        resourceType,
			LogicalName: string(urn.Name()),
			Reason:      "resource skipped via -skip-create",
		})
	}
	stubID := fmt.Sprintf("skip-%s", string(urn.Name()))
	return &pulumirpc.CreateResponse{
		Id:         stubID,
		Properties: req.GetProperties(),
	}, nil
}

func collectPropertyKeys(props resource.PropertyMap) []string {
	if len(props) == 0 {
		return nil
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	return keys
}
