package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var verbose int

// Execute runs the CLI.
func Execute() {
	rootCmd := newRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pulumi-tool-cdk-importer",
		Short: "Import CDK-managed resources into Pulumi state",
	}

	cmd.PersistentFlags().IntVarP(&verbose, "verbose", "v", 0, "Enable verbose logging (0-9)")
	cmd.AddCommand(newRuntimeCommand(), newProgramCommand())

	return cmd
}
