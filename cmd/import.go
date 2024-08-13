/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// importCmd represents the import command
var importCmd = &cobra.Command{
	Use:   "import",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("import called")
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			panic("--file must be provided")
		}
		outFile, _ := cmd.Flags().GetString("out")
		if info, err := os.Stat(outFile); err != nil && info == nil {
			os.Remove(outFile)
		}

		ccapi, err := newCcapi(cmd.Context())
		if err != nil {
			panic(err)
		}
		if err := ccapi.generateImportFile(cmd.Context(), file, outFile); err != nil {
			panic(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(importCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// importCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// importCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	importCmd.Flags().StringP("file", "f", "", "Location of the cdk import file")
	importCmd.Flags().StringP("out", "o", "pulumi-import.json", "File location to write the output to")
}
