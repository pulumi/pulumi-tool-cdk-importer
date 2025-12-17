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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pulumi/providertest/providers"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/imports"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/debug"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optremove"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
)

const (
	awsCCApi = "aws-native"
	aws      = "aws"
	docker   = "docker-build"
	// TODO: create workflow to update this
	awsVersion          = "7.14.0"
	awsCCApiVersion     = "1.40.0"
	dockerVersion       = "0.0.7"
	capturePassphrase   = "cdk-importer-local"
	providerWaitTimeout = 10 * time.Second
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
	Mode                   RunMode
	ImportFilePath         string
	Collector              *CaptureCollector
	SkipCreate             bool
	KeepImportState        bool
	LocalStackFile         string
	StackNames             []string
	Verbose                int
	UsePreviewImport       bool
	FilterPlaceholdersOnly bool
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

func RunPulumiUpWithProxies(ctx context.Context, logger *slog.Logger, lookups *lookups.Lookups, workDir string, opts RunOptions) error {
	if opts.Mode == CaptureImports && opts.ImportFilePath == "" {
		return fmt.Errorf("import file path is required when capturing imports")
	}
	collector := opts.Collector
	if collector == nil && opts.ImportFilePath != "" {
		collector = NewCaptureCollector()
	}
	if opts.Mode == CaptureImports && collector == nil {
		collector = NewCaptureCollector()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	logger.Info("Starting up providers...")
	envVars, stopProviders, err := startProxiedProviders(ctx, logger, lookups, pulumiTest{source: workDir}, opts, collector)
	if err != nil {
		return err
	}
	defer stopProviders()

	// Merge process environment into envVars
	// We iterate over os.Environ() and add any missing keys to envVars.
	// We prioritize the values returned by startProxiedProviders (which contains PULUMI_DEBUG_PROVIDERS).
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) == 2 {
			k, v := pair[0], pair[1]
			if _, exists := envVars[k]; !exists {
				envVars[k] = v
			}
		}
	}

	// Prevent Pulumi from checking for updates or new versions, which can cause hangs or delays
	envVars["PULUMI_SKIP_UPDATE_CHECK"] = "true"
	envVars["PULUMI_AUTOMATION_API_SKIP_VERSION_CHECK"] = "true"

	var stack auto.Stack
	var cleanup func()
	status := "unknown"
	resourcesImported := 0
	resourcesFailedToImport := 0
	primaryStack := ""
	if len(opts.StackNames) > 0 {
		primaryStack = opts.StackNames[0]
	}
	defer func() {
		importPath := opts.ImportFilePath
		importExists := false
		if importPath != "" {
			if _, err := os.Stat(importPath); err == nil {
				importExists = true
			}
		}
		l := logger.With(
			"status", status,
			"resourcesImported", resourcesImported,
			"resourcesFailedToImport", resourcesFailedToImport,
		)
		if primaryStack != "" {
			l = l.With("stack", primaryStack)
		}
		if importPath != "" {
			l = l.With("importFile", importPath, "importFileExists", importExists)
		}
		l.Info("Run complete")
	}()
	if opts.Mode == CaptureImports {
		stack, cleanup, err = prepareCaptureStack(ctx, logger, workDir, envVars, opts)
	} else {
		stack, err = prepareSelectedStack(ctx, workDir, envVars)
	}
	if err != nil {
		status = "failed"
		resourcesFailedToImport = 1
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	if err := stack.SetConfigWithOptions(ctx, "aws-native:autoNaming.autoTrim", auto.ConfigValue{Value: "true"}, &auto.ConfigOptions{
		Path: true,
	}); err != nil {
		status = "failed"
		resourcesFailedToImport = 1
		return fmt.Errorf("failed to set aws-native:autoNaming.autoTrim config: %w", err)
	}

	progressWriter := io.Discard
	errorWriter := io.Discard
	debugOptions := debug.LoggingOptions{}
	if opts.Verbose > 0 {
		level := uint(opts.Verbose)
		debugOptions.LogLevel = &level
		debugOptions.FlowToPlugins = true
		debugOptions.LogToStdErr = true
		debugOptions.Debug = true
		progressWriter = os.Stdout
		errorWriter = os.Stdout
	}
	var skeleton *imports.File
	if opts.ImportFilePath != "" && opts.UsePreviewImport {
		skeleton, err = runPreviewForImportFile(ctx, logger, stack, opts.ImportFilePath, debugOptions)
		if err != nil {
			return err
		}
	}

	eventCh := make(chan events.EngineEvent)
	eventTracker := newUpEventTracker()
	var eventWG sync.WaitGroup
	eventWG.Add(1)
	operationFailedErr := errors.New("operation failed")
	logUpErrors := func() {
		if summary := eventTracker.failureSummary(); summary != "" {
			logger.Info("Pulumi errors", "details", summary)
		}
	}
	go func() {
		defer eventWG.Done()
		eventTracker.consume(eventCh)
	}()

	logger.Info("Importing stack...")
	upErr := error(nil)
	_, upErr = stack.Up(ctx,
		optup.ContinueOnError(),
		optup.ProgressStreams(progressWriter),
		optup.ErrorProgressStreams(errorWriter),
		optup.DebugLogging(debugOptions),
		optup.EventStreams(eventCh),
		optup.SuppressProgress(),
	)
	eventWG.Wait()
	resourcesImported = eventTracker.created()
	resourcesFailedToImport = eventTracker.failedCreates()

	ensureFailureCount := func() {
		if upErr != nil && resourcesImported == 0 && resourcesFailedToImport == 0 {
			resourcesFailedToImport = 1
		}
	}

	if opts.ImportFilePath != "" {
		state, exportErr := stack.Export(ctx)
		if exportErr != nil {
			logger.Warn("Failed to export stack state", "error", exportErr)
			// If we can't export state, we'll still try to write what we captured
			state = apitype.UntypedDeployment{}
		}

		if upErr != nil {
			logger.Warn("pulumi up encountered errors, writing partial import file")
		}

		finalizeErr := finalizeCapture(logger, collector, opts.ImportFilePath, state, upErr != nil, skeleton, opts.FilterPlaceholdersOnly)
		if finalizeErr != nil {
			logger.Error("Error writing import file", "error", finalizeErr)
			// Return the finalize error if Up succeeded, otherwise return Up error
			if upErr == nil {
				status = "failed"
				ensureFailureCount()
				return finalizeErr
			}
		}

		// Return the original Up error if it occurred, so the command exits with error code
		if upErr != nil {
			status = "failed"
			ensureFailureCount()
			logUpErrors()
			return operationFailedErr
		}
	}

	if upErr != nil {
		logUpErrors()
		status = "failed"
		ensureFailureCount()
		return operationFailedErr
	}
	status = "success"
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

func prepareCaptureStack(ctx context.Context, logger *slog.Logger, workDir string, envVars map[string]string, opts RunOptions) (auto.Stack, func(), error) {
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
	logger.Info("Using capture stack", "stack", stackName, "backend", backendDir)
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
			logger.Warn("Failed to remove capture stack", "stack", stack.Name(), "error", err)
		}
		if createdTemp {
			if err := os.RemoveAll(backendDir); err != nil {
				logger.Warn("Failed to remove capture backend", "backend", backendDir, "error", err)
			}
		}
	}
	return stack, cleanup, nil
}

func resolveCaptureBackend(opts RunOptions) (string, string, bool, error) {
	stackName := deriveCaptureStackName(opts.StackNames, opts.LocalStackFile)
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

func deriveCaptureStackName(stackRefs []string, stackFile string) string {
	if stackFile != "" {
		base := strings.TrimSuffix(filepath.Base(stackFile), filepath.Ext(stackFile))
		if sanitized := sanitizeStackComponent(base); sanitized != "" {
			return sanitized
		}
	}
	if len(stackRefs) > 0 {
		var sanitizedParts []string
		for _, ref := range stackRefs {
			if sanitized := sanitizeStackComponent(ref); sanitized != "" {
				sanitizedParts = append(sanitizedParts, sanitized)
			}
		}
		if len(sanitizedParts) > 0 {
			return fmt.Sprintf("capture-%s", strings.Join(sanitizedParts, "-"))
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

func runPreviewForImportFile(ctx context.Context, logger *slog.Logger, stack auto.Stack, path string, debugOptions debug.LoggingOptions) (*imports.File, error) {
	if path == "" {
		return nil, fmt.Errorf("import file path is required for preview")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("ensuring import file directory: %w", err)
	}

	progressWriter := io.Discard
	errorWriter := io.Discard
	if debugOptions.Debug {
		progressWriter = os.Stdout
		errorWriter = os.Stdout
	}

	logger.Info("Running pulumi preview to generate import skeleton", "path", path)
	_, err := stack.Preview(
		ctx,
		optpreview.ImportFile(path),
		optpreview.DebugLogging(debugOptions),
		optpreview.ProgressStreams(progressWriter),
		optpreview.ErrorProgressStreams(errorWriter),
		optpreview.SuppressProgress(),
	)
	if err != nil {
		return nil, fmt.Errorf("pulumi preview for import file: %w", err)
	}

	file, err := imports.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading previewed import file %q: %w", path, err)
	}
	return file, nil
}

func finalizeCapture(logger *slog.Logger, collector *CaptureCollector, path string, deployment apitype.UntypedDeployment, isPartial bool, skeleton *imports.File, placeholdersOnly bool) error {
	if len(deployment.Deployment) == 0 {
		logger.Info("Exported stack deployment is empty; capture file will only include intercepted resources")
	} else {
		logger.Info("Exported stack deployment contains state", "bytes", len(deployment.Deployment))
	}

	if isPartial {
		logger.Info("Writing partial import file due to errors during execution")
	}
	summary := CaptureSummary{}
	entries := make([]Capture, 0)
	if collector != nil {
		summary = collector.Summary()
		entries = collector.Results()
	}
	if skeleton != nil {
		logger.Info("Merging preview import skeleton with captured resources", "count", len(entries))
	}
	captures := make([]imports.CaptureMetadata, 0, len(entries))
	for _, entry := range entries {
		captures = append(captures, imports.CaptureMetadata{
			Type:        entry.Type,
			Name:        entry.Name,
			LogicalName: entry.LogicalName,
			ID:          entry.ID,
			Properties:  append([]string(nil), entry.Properties...),
		})
	}
	file, err := imports.BuildFileFromDeployment(deployment, captures)
	if err != nil {
		return err
	}
	if skeleton != nil {
		file = imports.MergeWithSkeleton(skeleton, file)
	}
	if placeholdersOnly {
		originalCount := len(file.Resources)
		file = imports.FilterPlaceholderResources(file)
		filtered := len(file.Resources)
		switch {
		case filtered == 0:
			logger.Info("No placeholder resources found; import file will be empty")
		case filtered != originalCount:
			logger.Info("Filtered import file down to placeholder resources", "filtered", filtered, "original", originalCount)
		}
	}
	if err := imports.WriteFile(path, file); err != nil {
		return err
	}
	resultType := "complete"
	if isPartial {
		resultType = "partial"
	}
	logger.Info("Capture mode wrote resources",
		"resources", len(file.Resources),
		"path", path,
		"intercepts", summary.TotalIntercepts,
		"result", resultType,
	)
	if deduped := summary.TotalIntercepts - summary.UniqueResources; deduped > 0 {
		logger.Debug("Deduped duplicate captures", "count", deduped)
	}
	if len(summary.Skipped) > 0 {
		logger.Info("Skipped resources during capture", "count", len(summary.Skipped))
		for _, skipped := range summary.Skipped {
			logger.Info("Skipped resource", "logicalName", skipped.LogicalName, "type", skipped.Type, "reason", skipped.Reason)
		}
	}
	return nil
}

func sumResourceChanges(changes map[string]int) int {
	total := 0
	for _, v := range changes {
		if v > 0 {
			total += v
		}
	}
	return total
}

func startProxiedProviders(
	ctx context.Context,
	logger *slog.Logger,
	lookups *lookups.Lookups,
	pt providers.PulumiTest,
	opts RunOptions,
	collector *CaptureCollector,
) (map[string]string, func(), error) {
	providerLogger := logger.With("subcomponent", "providers")
	providerCtx, providerCancel := context.WithCancel(ctx)
	processes := &providerProcessSet{}

	ccapiBinary := newProviderFactory(awsCCApi, awsCCApiVersion, processes)
	ccapiIntercept := providers.ProviderInterceptFactory(providerCtx, ccapiBinary, awsCCApiInterceptors(lookups, opts, collector, providerLogger))
	awsBinary := newProviderFactory(aws, awsVersion, processes)
	awsIntercept := providers.ProviderInterceptFactory(providerCtx, awsBinary, awsInterceptors(lookups, opts, collector, providerLogger))
	dockerBinary := newProviderFactory(docker, dockerVersion, processes)
	dockerIntercept := providers.ProviderInterceptFactory(providerCtx, dockerBinary, dockerInterceptors())

	cleanup := func() {
		providerCancel()
		waitCtx, waitCancel := context.WithTimeout(context.Background(), providerWaitTimeout)
		defer waitCancel()
		processes.wait(waitCtx, providerLogger)
	}

	ps, err := providers.StartProviders(providerCtx, map[providers.ProviderName]providers.ProviderFactory{
		"aws-native":   ccapiIntercept,
		"aws":          awsIntercept,
		"docker-build": dockerIntercept,
	}, pt)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return map[string]string{
		"PULUMI_DEBUG_PROVIDERS": providers.GetDebugProvidersEnv(ps),
	}, cleanup, nil
}

func dockerInterceptors() providers.ProviderInterceptors {
	i := &dockerInterceptor{}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}

func awsInterceptors(lookups *lookups.Lookups, opts RunOptions, collector *CaptureCollector, logger *slog.Logger) providers.ProviderInterceptors {
	i := &awsInterceptor{
		Lookups:    lookups,
		mode:       opts.Mode,
		collector:  collector,
		skipCreate: opts.SkipCreate,
		logger:     logger.With("provider", "aws"),
	}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}

func awsCCApiInterceptors(lookups *lookups.Lookups, opts RunOptions, collector *CaptureCollector, logger *slog.Logger) providers.ProviderInterceptors {
	i := &awsCCApiInterceptor{
		Lookups:   lookups,
		mode:      opts.Mode,
		collector: collector,
		logger:    logger.With("provider", "aws-native"),
	}
	return providers.ProviderInterceptors{
		Create: i.create,
	}
}
