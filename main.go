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
)

func main() {
	ctx := context.Background()
	// var bucketIdRef = flag.String("bucket", "", "CDK logical ID for Bucket to import")
	// flag.Parse()
	var stackRef = flag.String("stack", "", "CloudFormation stack name")
	var classicProviderBinLocation = flag.String("classic-bin", "", "Location to the aws classic bin")
	flag.Parse()
	if stackRef == nil || *stackRef == "" {
		log.Fatalf("stack is required")
	}
	stackName := StackName(*stackRef)

	if classicProviderBinLocation == nil || *classicProviderBinLocation == "" {
		log.Fatalf("classic-bin is required")
	}

	classicBinLocation := AwsClassicBinLocation(*classicProviderBinLocation)

	cc, err := newCcapi(ctx)
	if err != nil {
		log.Fatal(err)
	}

	if err := cc.getStackResources(ctx, stackName); err != nil {
		log.Fatal(err)
	}

	if err := runPulumiUpWithProxies(ctx, cc, ".", classicBinLocation); err != nil {
		log.Fatal(err)
	}
}
