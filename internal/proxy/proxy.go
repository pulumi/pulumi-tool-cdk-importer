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
	"log"
	"os"
	"sort"

	"github.com/pulumi/providertest/providers"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/imports"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/debug"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

const (
	awsCCApi = "aws-native"
	aws      = "aws"
	docker   = "docker-build"
	// TODO: create workflow to update this
	awsVersion      = "7.11.0"
	awsCCApiVersion = "1.38.0"
	dockerVersion   = "0.0.7"
)

// RunMode determines how the proxied Pulumi run should behave.
type RunMode int

const (
	// RunPulumi executes a normal `pulumi up` with intercepted providers.
	RunPulumi RunMode = iota
	// CaptureImports will eventually capture primary IDs instead of mutating resources.
	CaptureImports
)

// RunOptions surfaces CLI decisions (mode, import path) into the proxy layer.
type RunOptions struct {
	Mode           RunMode
	ImportFilePath string
	Collector      *CaptureCollector
}

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

func RunPulumiUpWithProxies(ctx context.Context, logger *log.Logger, lookups *lookups.Lookups, workDir string, opts RunOptions) error {
	if opts.Mode == CaptureImports && opts.ImportFilePath == "" {
		return fmt.Errorf("import file path is required when capturing imports")
	}
	collector := opts.Collector
	if opts.Mode == CaptureImports && collector == nil {
		collector = NewCaptureCollector()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	logger.Println("Starting up providers...")
	envVars, err := startProxiedProviders(ctx, lookups, pulumiTest{source: workDir}, opts.Mode, collector)
	if err != nil {
		return err
	}
	ws, err := auto.NewLocalWorkspace(ctx, auto.WorkDir(workDir))
	if err != nil {
		return err
	}
	stack, err := ws.Stack(ctx)
	if err != nil || stack == nil {
		return fmt.Errorf("%w: make sure to select a stack with `pulumi stack select`", err)
	}
	s, err := auto.UpsertStackLocalSource(ctx, stack.Name, workDir, auto.EnvVars(envVars))
	if err != nil {
		return err
	}
	level := uint(1)
	logger.Println("Importing stack...")
	_, err = s.Up(ctx, optup.ContinueOnError(), optup.ProgressStreams(os.Stdout), optup.ErrorProgressStreams(os.Stdout), optup.DebugLogging(debug.LoggingOptions{
		LogLevel: &level,
	}))
	if err != nil {
		return err
	}
	if opts.Mode == CaptureImports {
		return finalizeCapture(logger, collector, opts.ImportFilePath)
	}
	return nil
}

func finalizeCapture(logger *log.Logger, collector *CaptureCollector, path string) error {
	summary := collector.Summary()
	entries := collector.Results()
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type == entries[j].Type {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Type < entries[j].Type
	})
	resources := make([]imports.Resource, 0, len(entries))
	for _, entry := range entries {
		resources = append(resources, imports.Resource{
			Type:        entry.Type,
			Name:        entry.Name,
			ID:          entry.ID,
			LogicalName: entry.LogicalName,
		})
	}
	file := &imports.File{Resources: resources}
	if err := imports.WriteFile(path, file); err != nil {
		return err
	}
	logger.Printf("Capture mode wrote %d resources to %s (intercepted %d create calls)", summary.UniqueResources, path, summary.TotalIntercepts)
	if deduped := summary.TotalIntercepts - summary.UniqueResources; deduped > 0 {
		logger.Printf("Deduped %d duplicate captures", deduped)
	}
	if len(summary.Skipped) > 0 {
		logger.Printf("Skipped %d resources during capture:", len(summary.Skipped))
		for _, skipped := range summary.Skipped {
			logger.Printf("  - %s (%s): %s", skipped.LogicalName, skipped.Type, skipped.Reason)
		}
	}
	return nil
}

func startProxiedProviders(
	ctx context.Context,
	lookups *lookups.Lookups,
	pt providers.PulumiTest,
	mode RunMode,
	collector *CaptureCollector,
) (map[string]string, error) {
	ccapiBinary := providers.DownloadPluginBinaryFactory(awsCCApi, awsCCApiVersion)
	ccapiIntercept := providers.ProviderInterceptFactory(ctx, ccapiBinary, awsCCApiInterceptors(lookups, mode, collector))
	awsBinary := providers.DownloadPluginBinaryFactory(aws, awsVersion)
	awsIntercept := providers.ProviderInterceptFactory(ctx, awsBinary, awsInterceptors(lookups, mode, collector))
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

func awsInterceptors(lookups *lookups.Lookups, mode RunMode, collector *CaptureCollector) providers.ProviderInterceptors {
	i := &awsInterceptor{Lookups: lookups, mode: mode, collector: collector}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}

func awsCCApiInterceptors(lookups *lookups.Lookups, mode RunMode, collector *CaptureCollector) providers.ProviderInterceptors {
	i := &awsCCApiInterceptor{Lookups: lookups, mode: mode, collector: collector}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}
