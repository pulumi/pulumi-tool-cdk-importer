package cmd

import (
	"encoding/json"
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

type importSpec struct {
	Type              tokens.Type `json:"type"`
	Name              string      `json:"name"`
	ID                resource.ID `json:"id,omitempty"`
	Parent            string      `json:"parent,omitempty"`
	Provider          string      `json:"provider,omitempty"`
	Version           string      `json:"version,omitempty"`
	PluginDownloadURL string      `json:"pluginDownloadUrl,omitempty"`
	Properties        []string    `json:"properties,omitempty"`
	Component         bool        `json:"component,omitempty"`
	Remote            bool        `json:"remote,omitempty"`

	// LogicalName is the resources Pulumi name (i.e. the first argument to `new Resource`).
	LogicalName string `json:"logicalName,omitempty"`
}

type importFile struct {
	NameTable map[string]resource.URN `json:"nameTable,omitempty"`
	Resources []importSpec            `json:"resources,omitempty"`
}

func readImportFile(p string) (importFile, error) {
	f, err := os.Open(p)
	if err != nil {
		return importFile{}, err
	}
	defer contract.IgnoreClose(f)

	var result importFile
	if err = json.NewDecoder(f).Decode(&result); err != nil {
		return importFile{}, err
	}
	return result, nil
}

func processImportFile(fileLocation string) error {
	// importFile, err := readImportFile(fileLocation)
	// if err != nil {
	// 	return err
	// }

	// var resourceTable map[string]importSpec = make(map[string]importSpec)
	//
	// for _, resource := range importFile.Resources {
	// }
	return nil
}
