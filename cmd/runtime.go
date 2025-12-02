package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/proxy"
)

func newRuntimeCommand() *cobra.Command {
	var stacks stringSlice
	var importFile string
	var skipCreate bool

	cmd := &cobra.Command{
		Use:   "runtime",
		Short: "Import resources from the pulumi-cdk runtime program in the current directory",
		RunE: func(_ *cobra.Command, _ []string) error {
			invocationDir, err := os.Getwd()
			if err != nil {
				return err
			}

			cfg := runConfig{
				mode:             proxy.RunPulumi,
				stacks:           stacks,
				importFile:       resolvePath(invocationDir, importFile),
				skipCreate:       skipCreate,
				workDir:          invocationDir,
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
	cmd.Flags().StringVar(&importFile, "import-file", "", "Path to write a Pulumi bulk import file after importing into the selected stack")
	cmd.Flags().BoolVar(&skipCreate, "skip-create", false, "Skip creation of special resources and only capture metadata")

	return cmd
}
