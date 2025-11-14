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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pulumi/providertest/providers"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/imports"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/debug"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optremove"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
)

const (
	awsCCApi = "aws-native"
	aws      = "aws"
	docker   = "docker-build"
	// TODO: create workflow to update this
	awsVersion        = "7.11.0"
	awsCCApiVersion   = "1.38.0"
	dockerVersion     = "0.0.7"
	capturePassphrase = "cdk-importer-local"
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
	Mode            RunMode
	ImportFilePath  string
	Collector       *CaptureCollector
	SkipCreate      bool
	KeepImportState bool
	LocalStackFile  string
	StackName       string
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
	envVars, err := startProxiedProviders(ctx, lookups, pulumiTest{source: workDir}, opts, collector)
	if err != nil {
		return err
	}
	var stack auto.Stack
	var cleanup func()
	if opts.Mode == CaptureImports {
		stack, cleanup, err = prepareCaptureStack(ctx, logger, workDir, envVars, opts)
	} else {
		stack, err = prepareSelectedStack(ctx, workDir, envVars)
	}
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	level := uint(1)
	logger.Println("Importing stack...")
	_, err = stack.Up(ctx, optup.ContinueOnError(), optup.ProgressStreams(os.Stdout), optup.ErrorProgressStreams(os.Stdout), optup.DebugLogging(debug.LoggingOptions{
		LogLevel: &level,
	}))
	if err != nil {
		return err
	}
	if opts.Mode == CaptureImports {
		state, err := stack.Export(ctx)
		if err != nil {
			return err
		}
		return finalizeCapture(logger, collector, opts.ImportFilePath, state)
	}
	return nil
}

func prepareSelectedStack(ctx context.Context, workDir string, envVars map[string]string) (auto.Stack, error) {
	var stack auto.Stack
	ws, err := auto.NewLocalWorkspace(ctx, auto.WorkDir(workDir), auto.EnvVars(envVars))
	if err != nil {
		return stack, err
	}
	summary, err := ws.Stack(ctx)
	if err != nil || summary == nil {
		return stack, fmt.Errorf("%w: make sure to select a stack with `pulumi stack select`", err)
	}
	return auto.UpsertStackLocalSource(ctx, summary.Name, workDir, auto.EnvVars(envVars))
}

func prepareCaptureStack(ctx context.Context, logger *log.Logger, workDir string, envVars map[string]string, opts RunOptions) (auto.Stack, func(), error) {
	var stack auto.Stack
	backendDir, stackName, createdTemp, err := resolveCaptureBackend(opts)
	if err != nil {
		return stack, nil, err
	}
	captureEnv := cloneEnv(envVars)
	captureEnv["PULUMI_BACKEND_URL"] = fmt.Sprintf("file://%s", backendDir)
	if _, ok := captureEnv["PULUMI_CONFIG_PASSPHRASE"]; !ok {
		captureEnv["PULUMI_CONFIG_PASSPHRASE"] = capturePassphrase
	}
	logger.Printf("Using capture stack %q with backend %s", stackName, backendDir)
	stack, err = auto.UpsertStackLocalSource(ctx, stackName, workDir, auto.EnvVars(captureEnv))
	if err != nil {
		if createdTemp {
			_ = os.RemoveAll(backendDir)
		}
		return stack, nil, err
	}
	cleanup := func() {
		if opts.LocalStackFile != "" || opts.KeepImportState {
			return
		}
		if err := stack.Workspace().RemoveStack(ctx, stack.Name(), optremove.Force()); err != nil {
			logger.Printf("failed to remove capture stack %s: %v", stack.Name(), err)
		}
		if createdTemp {
			if err := os.RemoveAll(backendDir); err != nil {
				logger.Printf("failed to remove capture backend %s: %v", backendDir, err)
			}
		}
	}
	return stack, cleanup, nil
}

func resolveCaptureBackend(opts RunOptions) (string, string, bool, error) {
	stackName := deriveCaptureStackName(opts.StackName, opts.LocalStackFile)
	if stackName == "" {
		stackName = fmt.Sprintf("capture-%d", time.Now().Unix())
	}
	if opts.LocalStackFile != "" {
		abs, err := filepath.Abs(opts.LocalStackFile)
		if err != nil {
			return "", "", false, err
		}
		dir := filepath.Dir(abs)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", "", false, err
		}
		return dir, stackName, false, nil
	}
	dir, err := os.MkdirTemp("", "pulumi-capture-")
	if err != nil {
		return "", "", false, err
	}
	return dir, stackName, true, nil
}

func deriveCaptureStackName(stackRef, stackFile string) string {
	if stackFile != "" {
		base := strings.TrimSuffix(filepath.Base(stackFile), filepath.Ext(stackFile))
		if sanitized := sanitizeStackComponent(base); sanitized != "" {
			return sanitized
		}
	}
	if stackRef != "" {
		if sanitized := sanitizeStackComponent(stackRef); sanitized != "" {
			return fmt.Sprintf("capture-%s", sanitized)
		}
	}
	return ""
}

func sanitizeStackComponent(value string) string {
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-. ")
}

func cloneEnv(env map[string]string) map[string]string {
	if env == nil {
		return map[string]string{}
	}
	dup := make(map[string]string, len(env))
	for k, v := range env {
		dup[k] = v
	}
	return dup
}

func finalizeCapture(logger *log.Logger, collector *CaptureCollector, path string, deployment apitype.UntypedDeployment) error {
	if len(deployment.Deployment) == 0 {
		logger.Println("Exported stack deployment is empty; capture file will only include intercepted resources")
	} else {
		logger.Printf("Exported stack deployment contains %d bytes of state", len(deployment.Deployment))
	}
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
	opts RunOptions,
	collector *CaptureCollector,
) (map[string]string, error) {
	ccapiBinary := providers.DownloadPluginBinaryFactory(awsCCApi, awsCCApiVersion)
	ccapiIntercept := providers.ProviderInterceptFactory(ctx, ccapiBinary, awsCCApiInterceptors(lookups, opts, collector))
	awsBinary := providers.DownloadPluginBinaryFactory(aws, awsVersion)
	awsIntercept := providers.ProviderInterceptFactory(ctx, awsBinary, awsInterceptors(lookups, opts, collector))
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

func awsInterceptors(lookups *lookups.Lookups, opts RunOptions, collector *CaptureCollector) providers.ProviderInterceptors {
	i := &awsInterceptor{Lookups: lookups, mode: opts.Mode, collector: collector, skipCreate: opts.SkipCreate}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}

func awsCCApiInterceptors(lookups *lookups.Lookups, opts RunOptions, collector *CaptureCollector) providers.ProviderInterceptors {
	i := &awsCCApiInterceptor{Lookups: lookups, mode: opts.Mode, collector: collector}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}
