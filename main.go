package main

import (
	"context"
	//"flag"
	"fmt"
	"log"
)

func main() {
	// var bucketIdRef = flag.String("bucket", "", "CDK logical ID for Bucket to import")
	// flag.Parse()

	ctx := context.Background()
	if err := runPulumiUpWithProxies(ctx, "."); err != nil {
		log.Fatal(err)
	}
}

func debugDiscoverCDKLogicalResourceID(bucketId CDKLogicalResourceID) {
	// bucketId := CDKLogicalResourceID(*bucketIdRef)
	ctx := context.Background()
	c, err := NewCloudControlClient(ctx)
	if err != nil {
		panic(err)
	}
	results, err := DiscoverByCDKLogicalResourceID(ctx, c, "AWS::S3::Bucket", bucketId)
	if err != nil {
		panic(err)
	}
	for _, r := range results {
		fmt.Println(r)
	}
}
