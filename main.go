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

package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/proxy"
)

func main() {
	logger := log.New(os.Stdout, "[cdk-importer] ", log.Ltime|log.Lshortfile)
	ctx := context.Background()
	var stackRef = flag.String("stack", "", "CloudFormation stack name")
	var importFile = flag.String("import-file", "", "If set, capture resource IDs into this Pulumi bulk import file path")
	var skipCreate = flag.Bool("skip-create", false, "Skip creation of special resources and only capture metadata")
	var keepImportState = flag.Bool("keep-import-state", false, "Keep the temporary local backend after capture runs finish")
	var localStackFile = flag.String("local-stack-file", "", "Path to the local backend file to re-use when capturing imports")
	flag.Parse()
	if stackRef == nil || *stackRef == "" {
		log.Fatalf("stack is required")
	}
	stackName := common.StackName(*stackRef)

	mode := proxy.RunPulumi
	var importPath string
	skipCreateMode := skipCreate != nil && *skipCreate
	keepState := keepImportState != nil && *keepImportState
	localStack := ""
	if localStackFile != nil {
		localStack = *localStackFile
	}
	if importFile != nil && *importFile != "" {
		mode = proxy.CaptureImports
		importPath = *importFile
		skipCreateMode = true
	} else {
		if keepState {
			log.Fatalf("-keep-import-state requires -import-file to be set")
		}
		if localStack != "" {
			log.Fatalf("-local-stack-file requires -import-file to be set")
		}
	}

	cc, err := lookups.NewDefaultLookups(ctx)
	if err != nil {
		logger.Fatal(err)
	}

	logger.Printf("Getting stack resources for stack: %s", stackName)
	if err := cc.GetStackResources(ctx, stackName); err != nil {
		logger.Fatal(err)
	}

	options := proxy.RunOptions{
		Mode:            mode,
		ImportFilePath:  importPath,
		SkipCreate:      skipCreateMode,
		KeepImportState: keepState,
		LocalStackFile:  localStack,
	}

	if err := proxy.RunPulumiUpWithProxies(ctx, logger, cc, ".", options); err != nil {
		log.Fatal(err)
	}
}
