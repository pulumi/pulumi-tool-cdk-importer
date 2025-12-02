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
	var skipCreate bool

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
				mode:             proxy.RunPulumi,
				stacks:           stacks,
				importFile:       resolvePath(invocationDir, importFile),
				skipCreate:       skipCreate,
				workDir:          workDir,
				invocationDir:    invocationDir,
				keepImportState:  false,
				localStackFile:   "",
				usePreviewImport: false,
				verbose:          verbose,
			}
			return run(cfg)
		},
	}

	cmd.Flags().Var(&stacks, "stack", "CloudFormation stack name (can be specified multiple times or comma-separated)")
	_ = cmd.MarkFlagRequired("stack")
	cmd.Flags().StringVar(&programDir, "program-dir", "", "Path to an existing Pulumi program generated from a CDK app")
	_ = cmd.MarkFlagRequired("program-dir")
	cmd.Flags().StringVar(&importFile, "import-file", "", "Path to write a Pulumi bulk import file after importing into the selected stack")
	cmd.Flags().BoolVar(&skipCreate, "skip-create", false, "Skip creation of special resources and only capture metadata")

	return cmd
}

func newProgramIterateCommand() *cobra.Command {
	var stacks stringSlice
	var programDir string
	var importFile string
	var keepImportState bool
	var localStackFile string

	cmd := &cobra.Command{
		Use:   "iterate",
		Short: "Iterate on imports using a local backend and import file capture",
		RunE: func(_ *cobra.Command, _ []string) error {
			if programDir == "" {
				return fmt.Errorf("--program-dir is required")
			}
			if importFile == "" {
				return fmt.Errorf("--import-file is required when using iterate mode")
			}
			invocationDir, err := os.Getwd()
			if err != nil {
				return err
			}
			workDir := resolvePath(invocationDir, programDir)
			cfg := runConfig{
				mode:             proxy.CaptureImports,
				stacks:           stacks,
				importFile:       resolvePath(invocationDir, importFile),
				skipCreate:       true,
				workDir:          workDir,
				invocationDir:    invocationDir,
				keepImportState:  keepImportState,
				localStackFile:   resolvePath(invocationDir, localStackFile),
				usePreviewImport: true,
				verbose:          verbose,
			}
			return run(cfg)
		},
	}

	cmd.Flags().Var(&stacks, "stack", "CloudFormation stack name (can be specified multiple times or comma-separated)")
	_ = cmd.MarkFlagRequired("stack")
	cmd.Flags().StringVar(&programDir, "program-dir", "", "Path to an existing Pulumi program generated from a CDK app")
	_ = cmd.MarkFlagRequired("program-dir")
	cmd.Flags().StringVar(&importFile, "import-file", "", "Path to write a Pulumi bulk import file")
	_ = cmd.MarkFlagRequired("import-file")
	cmd.Flags().BoolVar(&keepImportState, "keep-import-state", false, "Keep the temporary local backend after capture runs finish")
	cmd.Flags().StringVar(&localStackFile, "local-stack-file", "", "Path to the local backend file to re-use when capturing imports")

	return cmd
}
