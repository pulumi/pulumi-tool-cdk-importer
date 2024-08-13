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
	flag.Parse()
	if stackRef == nil || *stackRef == "" {
		log.Fatalf("stack is required")
	}
	stackName := StackName(*stackRef)

	cc, err := newCcapi(ctx)
	if err != nil {
		log.Fatal(err)
	}

	if err := cc.getStackResources(ctx, stackName); err != nil {
		log.Fatal(err)
	}

	if err := runPulumiUpWithProxies(ctx, cc, "."); err != nil {
		log.Fatal(err)
	}
}
