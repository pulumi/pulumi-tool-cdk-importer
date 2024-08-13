/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// genTypesCmd represents the genTypes command
var genTypesCmd = &cobra.Command{
	Use:   "genTypes",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("genTypes called")
		folder, _ := cmd.Flags().GetString("folder")
		if folder == "" {
			f, _ := os.Getwd()
			folder = f
		}
		outFile, _ := cmd.Flags().GetString("out")
		if info, err := os.Stat(outFile); err != nil && info.Name() != "" {
			os.Remove(outFile)
		}

		entries, err := os.ReadDir(folder)
		if err != nil {
			panic(err)
		}

		var mapping map[string]schemaInfo = make(map[string]schemaInfo)
		for _, entry := range entries {
			if entry.Type().IsRegular() {
				contents, err := os.ReadFile(filepath.Join(folder, entry.Name()))
				if err != nil {
					panic(err)
				}
				var schema map[string]interface{}
				if err := json.Unmarshal(contents, &schema); err != nil {
					panic(err)
				}
				name, res, err := processSchemaJson(schema)
				if err != nil {
					panic(err)
				}

				mapping[name] = res
			}
		}

		contents, err := json.MarshalIndent(mapping, "", "\t")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(outFile, contents, 0777); err != nil {
			panic(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(genTypesCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// genTypesCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// genTypesCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	genTypesCmd.Flags().StringP("folder", "f", "", "Location to the cloudformation resource schemas")
	genTypesCmd.Flags().StringP("out", "o", "cfn-schema.json", "File location to write the output to")
}

type identifierType string

const (
	identifierType_INPUT  identifierType = "INPUT"
	identifierType_OUTPUT identifierType = "OUTPUT"
)

type IdentifierInfo struct {
	Name           string         `json:"name"`
	IdentifierType identifierType `json:"identifierType"`
}

type schemaInfo []IdentifierInfo

func processSchemaJson(schema map[string]interface{}) (string, schemaInfo, error) {
	typeName := schema["typeName"].(string)
	var createOnlyProperties []interface{}
	var readOnlyProperties []interface{}
	primaryIds, ok := schema["primaryIdentifier"].([]interface{})
	if !ok {
		fmt.Println(schema)
		return "", nil, fmt.Errorf("%s: primaryIdentifier doesn't exist", typeName)
	}
	cp, ok := schema["createOnlyProperties"]
	if ok {
		createOnlyProperties = cp.([]interface{})
	}
	rp, ok := schema["readOnlyProperties"]
	if ok {
		readOnlyProperties = rp.([]interface{})

	}

	infos := schemaInfo{}
	var idType identifierType
	for _, name := range primaryIds {
		stringName := name.(string)
		if slices.Contains(createOnlyProperties, name) {
			idType = identifierType_INPUT
		} else if slices.Contains(readOnlyProperties, name) {
			idType = identifierType_OUTPUT
		} else {
			// otherwise it's both so treat it as an input
			idType = identifierType_INPUT
		}
		nameParts := strings.Split(stringName, "/")[2]
		infos = append(infos, IdentifierInfo{
			Name:           nameParts,
			IdentifierType: idType,
		})
	}
	return typeName, infos, nil

}
