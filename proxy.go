package main

import (
	"context"
	"fmt"

	"github.com/pulumi/providertest/providers"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	awsNative        = "aws-native"
	awsNativeVersion = "0.116.0"
)

func runPulumiUpWithProxies(ctx context.Context, c *ccapi, workDir string) error {
	envVars, err := startProxiedProviders(ctx, c)
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

func startProxiedProviders(ctx context.Context, c *ccapi) (map[string]string, error) {
	f1 := providers.DownloadPluginBinaryFactory(awsNative, awsNativeVersion)
	f2 := providers.ProviderInterceptFactory(ctx, f1, awsNativeInterceptors(c))
	ps, err := providers.StartProviders(ctx, map[providers.ProviderName]providers.ProviderFactory{
		"aws-native": f2,
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"PULUMI_DEBUG_PROVIDERS": providers.GetDebugProvidersEnv(ps),
	}, nil
}

func awsNativeInterceptors(c *ccapi) providers.ProviderInterceptors {
	i := &awsNativeInterceptor{c}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
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

	logical, err := c.findLogicalResourceID(ctx, urn)
	if err != nil {
		return nil, err
	}
	prim, err := c.findPrimaryResourceID(ctx, urn.Type(), logical, props)
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
