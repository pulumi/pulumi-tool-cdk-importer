package main

import (
	"context"
	"fmt"

	"github.com/pulumi/providertest/providers"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

const (
	awsNative        = "aws-native"
	awsNativeVersion = "0.116.0"
)

func runPulumiUpWithProxies(ctx context.Context, workDir string) error {
	envVars, err := startProxiedProviders(ctx)
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
	_, err = s.Up(ctx)
	if err != nil {
		return err
	}
	return nil
}

func startProxiedProviders(ctx context.Context) (map[string]string, error) {
	f1 := providers.DownloadPluginBinaryFactory(awsNative, awsNativeVersion)
	f2 := providers.ProviderInterceptFactory(ctx, f1, awsNativeInterceptors())
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

func awsNativeInterceptors() providers.ProviderInterceptors {
	return providers.ProviderInterceptors{
		Create: func(
			ctx context.Context,
			in *pulumirpc.CreateRequest,
			client pulumirpc.ResourceProviderClient,
		) (*pulumirpc.CreateResponse, error) {
			return nil, fmt.Errorf("INTERCEPTED CREATE")
		},
	}
}
