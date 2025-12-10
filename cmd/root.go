package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var verbose int
var debugLogging bool

// Execute runs the CLI.
func Execute() {
	rootCmd := newRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, formatCLIError(err))
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pulumi-tool-cdk-importer",
		Short: "Import CDK-managed resources into Pulumi state",
	}
	// We render errors ourselves to avoid Cobra printing them twice along with usage text.
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	cmd.PersistentFlags().IntVarP(&verbose, "verbose", "v", 0, "Enable verbose logging (0-9)")
	cmd.PersistentFlags().BoolVar(&debugLogging, "debug", false, "Enable debug-level logging for the importer")
	cmd.AddCommand(newRuntimeCommand(), newProgramCommand())

	return cmd
}
