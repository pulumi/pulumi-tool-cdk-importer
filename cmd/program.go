package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/proxy"
)

func newProgramCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "program",
		Short: "Operate on an existing Pulumi program generated from a CDK app",
	}

	cmd.AddCommand(newProgramImportCommand(), newProgramIterateCommand())
	return cmd
}

func newProgramImportCommand() *cobra.Command {
	var stacks stringSlice
	var programDir string
	var importFile string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import into the selected stack using an existing Pulumi program",
		RunE: func(_ *cobra.Command, _ []string) error {
			if programDir == "" {
				return fmt.Errorf("--program-dir is required")
			}
			invocationDir, err := os.Getwd()
			if err != nil {
				return err
			}
			workDir := resolvePath(invocationDir, programDir)
			cfg := runConfig{
				mode:            proxy.RunPulumi,
				stacks:          stacks,
				importFile:      resolvePath(invocationDir, importFile),
				skipCreate:      true,
				workDir:         workDir,
				invocationDir:   invocationDir,
				keepImportState: false,
				localStackFile:  "",
				debugLogging:    debugLogging,
				verbose:         verbose,
			}
			return run(cfg)
		},
	}

	cmd.Flags().Var(&stacks, "stack", "CloudFormation stack name (can be specified multiple times or comma-separated)")
	_ = cmd.MarkFlagRequired("stack")
	cmd.Flags().StringVar(&programDir, "program-dir", "", "Path to an existing Pulumi program generated from a CDK app")
	_ = cmd.MarkFlagRequired("program-dir")
	cmd.Flags().StringVar(&importFile, "import-file", "", "Path to write a Pulumi bulk import file after importing into the selected stack (default: import.json when provided without a value)")
	cmd.Flags().Lookup("import-file").NoOptDefVal = defaultImportFileName

	return cmd
}

func newProgramIterateCommand() *cobra.Command {
	var stacks stringSlice
	var programDir string
	var importFile string

	cmd := &cobra.Command{
		Use:   "iterate",
		Short: "Iterate on imports using a local backend and import file capture",
		RunE: func(_ *cobra.Command, _ []string) error {
			if programDir == "" {
				return fmt.Errorf("--program-dir is required")
			}
			invocationDir, err := os.Getwd()
			if err != nil {
				return err
			}
			workDir := resolvePath(invocationDir, programDir)
			resolvedImport := importFile
			if resolvedImport == "" {
				resolvedImport = defaultImportFileName
			}
			cfg := runConfig{
				mode:            proxy.CaptureImports,
				stacks:          stacks,
				importFile:      resolvePath(invocationDir, resolvedImport),
				skipCreate:      true,
				workDir:         workDir,
				invocationDir:   invocationDir,
				keepImportState: true,
				localStackFile:  resolvePath(invocationDir, defaultLocalStackFile),
				debugLogging:    debugLogging,
				verbose:         verbose,
			}
			return run(cfg)
		},
	}

	cmd.Flags().Var(&stacks, "stack", "CloudFormation stack name (can be specified multiple times or comma-separated)")
	_ = cmd.MarkFlagRequired("stack")
	cmd.Flags().StringVar(&programDir, "program-dir", "", "Path to an existing Pulumi program generated from a CDK app")
	_ = cmd.MarkFlagRequired("program-dir")
	cmd.Flags().StringVar(&importFile, "import-file", "", "Path to write a Pulumi bulk import file (default: import.json when omitted or provided without a value)")
	cmd.Flags().Lookup("import-file").NoOptDefVal = defaultImportFileName

	return cmd
}
