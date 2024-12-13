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

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/lookups"
	"github.com/pulumi/pulumi-tool-cdk-importer/internal/proxy"
)

func main() {
	ctx := context.Background()
	var stackRef = flag.String("stack", "", "CloudFormation stack name")
	flag.Parse()
	if stackRef == nil || *stackRef == "" {
		log.Fatalf("stack is required")
	}
	stackName := common.StackName(*stackRef)

	cc, err := lookups.NewCCApiLookups(ctx)
	if err != nil {
		log.Fatal(err)
	}

	if err := cc.GetStackResources(ctx, stackName); err != nil {
		log.Fatal(err)
	}

	if err := proxy.RunPulumiUpWithProxies(ctx, cc, "."); err != nil {
		log.Fatal(err)
	}
}
