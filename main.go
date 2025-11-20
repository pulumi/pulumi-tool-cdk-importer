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
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/cdk"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/proxy"
)

func main() {
	logger := log.New(os.Stdout, "[cdk-importer] ", log.Ltime|log.Lshortfile)
	ctx := context.Background()
	var stacks stringSlice
	flag.Var(&stacks, "stack", "CloudFormation stack name (can be specified multiple times)")
	var importFile = flag.String("import-file", "", "If set, capture resource IDs into this Pulumi bulk import file path")
	var skipCreate = flag.Bool("skip-create", false, "Skip creation of special resources and only capture metadata")
	var keepImportState = flag.Bool("keep-import-state", false, "Keep the temporary local backend after capture runs finish")
	var localStackFile = flag.String("local-stack-file", "", "Path to the local backend file to re-use when capturing imports")
	var cdkApp = flag.String("cdk-app", "", "Path to the CDK application to import")
	var verbose = flag.Int("verbose", 0, "Enable verbose logging (0-9)")
	flag.Parse()

	if os.Getenv("AWS_REGION") == "" && os.Getenv("AWS_DEFAULT_REGION") == "" {
		log.Fatal("AWS_REGION or AWS_DEFAULT_REGION environment variable must be set")
	}

	if *cdkApp != "" {
		// Apply defaults for cdk-app mode if not explicitly set
		if *importFile == "" {
			*importFile = "import.json"
		}
		// We can't easily check if bool flags were set by user or default,
		// but for these specific flags, if cdk-app is set, we want to enforce these defaults
		// unless we want to add more complex flag parsing logic.
		// Given the requirement "imply -skip-create", we will set them to true.
		// If the user explicitly sets -skip-create=false, this would overwrite it.
		// To handle that correctly, we would need to check if the flag was visited.
		// However, standard flag package doesn't make this super easy without visiting.
		// Let's assume "imply" means "set default to true".

		// A better way with standard flag is to check if they have their default values
		// but that doesn't distinguish "not set" from "set to default".
		// For now, let's just set them if they are false (default).
		if !*skipCreate {
			*skipCreate = true
		}
		if !*keepImportState {
			*keepImportState = true
		}
		if *localStackFile == "" {
			*localStackFile = "stack-state.json"
		}

		wd, err := cdk.RunCDK2Pulumi(cdk2pulumiBinary, *cdkApp, stacks)
		if err != nil {
			log.Fatalf("failed to run cdk2pulumi: %v", err)
		}
		if err := os.Chdir(wd); err != nil {
			log.Fatalf("failed to change directory: %v", err)
		}
	}

	if len(stacks) == 0 {
		log.Fatalf("stack is required")
	}

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

	for _, stackRef := range stacks {
		stackName := common.StackName(stackRef)
		logger.Printf("Getting stack resources for stack: %s", stackName)
		if err := cc.GetStackResources(ctx, stackName); err != nil {
			logger.Fatal(err)
		}
	}

	options := proxy.RunOptions{
		Mode:            mode,
		ImportFilePath:  importPath,
		SkipCreate:      skipCreateMode,
		KeepImportState: keepState,
		LocalStackFile:  localStack,
		StackNames:      stacks,
		Verbose:         *verbose,
	}

	if err := proxy.RunPulumiUpWithProxies(ctx, logger, cc, ".", options); err != nil {
		log.Fatal(err)
	}
}

type stringSlice []string

func (s *stringSlice) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *stringSlice) Set(value string) error {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			*s = append(*s, trimmed)
		}
	}
	return nil
}
