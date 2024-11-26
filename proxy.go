// Copyright 2016-2024, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/pulumi/providertest/providers"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

const (
	awsNative        = "aws-native"
	aws              = "aws"
	awsVersion       = "6.49.0"
	awsNativeVersion = "0.116.0"
)

func runPulumiUpWithProxies(ctx context.Context, c *ccapi, workDir string, classicBin AwsClassicBinLocation) error {
	envVars, err := startProxiedProviders(ctx, c, classicBin)
	if err != nil {
		return err
	}
	ws, err := auto.NewLocalWorkspace(ctx, auto.WorkDir(workDir))
	if err != nil {
		return err
	}
	stack, err := ws.Stack(ctx)
	if err != nil {
		return nil
	}
	s, err := auto.UpsertStackLocalSource(ctx, stack.Name, workDir, auto.EnvVars(envVars))
	if err != nil {
		return err
	}
	_, err = s.Up(ctx, optup.ContinueOnError())
	if err != nil {
		return err
	}
	return nil
}

func startProxiedProviders(ctx context.Context, c *ccapi, classicBin AwsClassicBinLocation) (map[string]string, error) {
	native1 := providers.DownloadPluginBinaryFactory(awsNative, awsNativeVersion)
	native2 := providers.ProviderInterceptFactory(ctx, native1, awsNativeInterceptors(c))
	classic1 := providers.LocalBinary(aws, string(classicBin))
	classic2 := providers.ProviderInterceptFactory(ctx, classic1, awsClassicInterceptors(c))
	ps, err := providers.StartProviders(ctx, map[providers.ProviderName]providers.ProviderFactory{
		"aws-native": native2,
		"aws":        classic2,
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"PULUMI_DEBUG_PROVIDERS": providers.GetDebugProvidersEnv(ps),
	}, nil
}

func awsClassicInterceptors(c *ccapi) providers.ProviderInterceptors {
	i := &awsClassicInterceptor{c}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}

func awsNativeInterceptors(c *ccapi) providers.ProviderInterceptors {
	i := &awsNativeInterceptor{c}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}

type awsClassicInterceptor struct {
	c *ccapi
}

func (i *awsClassicInterceptor) create(
	ctx context.Context,
	in *pulumirpc.CreateRequest,
	client pulumirpc.ResourceProviderClient,
) (*pulumirpc.CreateResponse, error) {
	c := i.c
	urn, err := resource.ParseURN(in.GetUrn())
	if err != nil {
		return nil, err
	}
	resourceType := string(urn.Type())
	// bucket objects are a special case. CDK doesn't have a resource for them, they are handled
	// by cdk assets which is a cli tool. pulumi-cdk converts them into BucketObjectV2 so go ahead
	// and just create the object
	if resourceType == "aws:s3/bucketObjectv2:BucketObjectv2" {
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
	logical, err := c.findLogicalResourceID(ctx, urn, awsClassicMetadata)
	if err != nil {
		return nil, err
	}
	prim, err := c.findClassicPrimaryResourceID(ctx, urn.Type(), logical, inputs.Mappable())
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

type awsNativeInterceptor struct {
	c *ccapi
}

func (i *awsNativeInterceptor) create(
	ctx context.Context,
	in *pulumirpc.CreateRequest,
	client pulumirpc.ResourceProviderClient,
) (*pulumirpc.CreateResponse, error) {
	c := i.c
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

	props, err := awsNativeMetadata.CfnProperties(resourceToken, inputs)
	if err != nil {
		return nil, err
	}

	logical, err := c.findLogicalResourceID(ctx, urn, awsNativeMetadata)
	if err != nil {
		return nil, err
	}
	prim, err := c.findNativePrimaryResourceID(ctx, urn.Type(), logical, props)
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
