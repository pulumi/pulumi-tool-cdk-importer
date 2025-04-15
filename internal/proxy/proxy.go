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

package proxy

import (
	"context"
	"fmt"
	"os"

	"github.com/pulumi/providertest/providers"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/debug"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

const (
	awsCCApi        = "aws-native"
	aws             = "aws"
	docker          = "docker-build"
	awsVersion      = "6.76.0"
	awsCCApiVersion = "1.26.0"
	dockerVersion   = "0.0.7"
)

type pulumiTest struct {
	source string
}

func (pt pulumiTest) Source() string {
	return pt.source
}

type ProxiesConfig struct {
	Region            string
	Account           string
	CfnStackResources map[common.LogicalResourceID]lookups.CfnStackResource
}

func RunPulumiUpWithProxies(ctx context.Context, lookups *lookups.Lookups, workDir string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	envVars, err := startProxiedProviders(ctx, lookups, pulumiTest{source: workDir})
	if err != nil {
		return err
	}
	ws, err := auto.NewLocalWorkspace(ctx, auto.WorkDir(workDir))
	if err != nil {
		return err
	}
	stack, err := ws.Stack(ctx)
	if err != nil || stack == nil {
		return fmt.Errorf("failed to find a current pulumi stack. Make sure to select a stack with 'pulumi stack select': %w", err)
	}
	s, err := auto.UpsertStackLocalSource(ctx, stack.Name, workDir, auto.EnvVars(envVars))
	if err != nil {
		return err
	}
	level := uint(1)
	_, err = s.Up(ctx, optup.ContinueOnError(), optup.ProgressStreams(os.Stdout), optup.ErrorProgressStreams(os.Stdout), optup.DebugLogging(debug.LoggingOptions{
		LogLevel: &level,
	}))
	if err != nil {
		return err
	}
	return nil
}

func startProxiedProviders(
	ctx context.Context,
	lookups *lookups.Lookups,
	pt providers.PulumiTest,
) (map[string]string, error) {
	ccapiBinary := providers.DownloadPluginBinaryFactory(awsCCApi, awsCCApiVersion)
	ccapiIntercept := providers.ProviderInterceptFactory(ctx, ccapiBinary, awsCCApiInterceptors(lookups))
	awsBinary := providers.DownloadPluginBinaryFactory(aws, awsVersion)
	awsIntercept := providers.ProviderInterceptFactory(ctx, awsBinary, awsInterceptors(lookups))
	dockerBinary := providers.DownloadPluginBinaryFactory(docker, dockerVersion)
	dockerIntercept := providers.ProviderInterceptFactory(ctx, dockerBinary, dockerInterceptors())
	ps, err := providers.StartProviders(ctx, map[providers.ProviderName]providers.ProviderFactory{
		"aws-native":   ccapiIntercept,
		"aws":          awsIntercept,
		"docker-build": dockerIntercept,
	}, pt)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"PULUMI_DEBUG_PROVIDERS": providers.GetDebugProvidersEnv(ps),
	}, nil
}

func dockerInterceptors() providers.ProviderInterceptors {
	i := &dockerInterceptor{}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}

func awsInterceptors(lookups *lookups.Lookups) providers.ProviderInterceptors {
	i := &awsInterceptor{lookups}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}

func awsCCApiInterceptors(lookups *lookups.Lookups) providers.ProviderInterceptors {
	i := &awsCCApiInterceptor{lookups}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}
