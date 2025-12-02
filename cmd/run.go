package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/proxy"
)

type runConfig struct {
	mode             proxy.RunMode
	stacks           []string
	importFile       string
	skipCreate       bool
	keepImportState  bool
	localStackFile   string
	workDir          string
	invocationDir    string
	usePreviewImport bool
	verbose          int
}

func run(cfg runConfig) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}

	logger := log.New(os.Stdout, "[cdk-importer] ", log.Ltime|log.Lshortfile)
	ctx := context.Background()

	if err := os.Chdir(cfg.workDir); err != nil {
		return fmt.Errorf("failed to change directory to program: %w", err)
	}

	cc, err := lookups.NewDefaultLookups(ctx)
	if err != nil {
		return err
	}

	for _, stackRef := range cfg.stacks {
		stackName := common.StackName(stackRef)
		logger.Printf("Getting stack resources for stack: %s", stackName)
		if err := cc.GetStackResources(ctx, stackName); err != nil {
			return err
		}
	}

	mode := cfg.mode
	importPath := cfg.importFile
	skipCreateMode := cfg.skipCreate
	keepState := cfg.keepImportState
	localStack := cfg.localStackFile

	if mode == proxy.CaptureImports {
		skipCreateMode = true
	}

	options := proxy.RunOptions{
		Mode:             mode,
		ImportFilePath:   importPath,
		SkipCreate:       skipCreateMode,
		KeepImportState:  keepState,
		LocalStackFile:   localStack,
		StackNames:       cfg.stacks,
		Verbose:          cfg.verbose,
		UsePreviewImport: cfg.usePreviewImport,
	}

	return proxy.RunPulumiUpWithProxies(ctx, logger, cc, ".", options)
}

func validateConfig(cfg runConfig) error {
	if os.Getenv("AWS_REGION") == "" && os.Getenv("AWS_DEFAULT_REGION") == "" {
		return fmt.Errorf("AWS_REGION or AWS_DEFAULT_REGION environment variable must be set")
	}
	if len(cfg.stacks) == 0 {
		return fmt.Errorf("stack is required")
	}
	if cfg.workDir == "" {
		return fmt.Errorf("program directory is required")
	}
	if cfg.mode == proxy.CaptureImports && cfg.importFile == "" {
		return fmt.Errorf("--import-file is required in iterate mode")
	}
	if cfg.mode == proxy.RunPulumi {
		if cfg.keepImportState {
			return fmt.Errorf("--keep-import-state is only supported in iterate mode")
		}
		if cfg.localStackFile != "" {
			return fmt.Errorf("--local-stack-file is only supported in iterate mode")
		}
	}
	return nil
}

func resolvePath(baseDir, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
