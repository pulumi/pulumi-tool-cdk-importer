package proxy

import (
	"context"
	"testing"

	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

func TestAWSInterceptorSkipCreate(t *testing.T) {
	t.Parallel()

	interceptor := &awsInterceptor{
		mode:       RunPulumi,
		skipCreate: true,
		collector:  NewCaptureCollector(),
	}

	req := &pulumirpc.CreateRequest{
		Urn: "urn:pulumi:test::proj::aws:s3/bucketPolicy:BucketPolicy::example",
	}

	resp, err := interceptor.create(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if resp == nil || resp.Id == "" {
		t.Fatalf("expected stub response with ID, got %#v", resp)
	}

	summary := interceptor.collector.Summary()
	if len(summary.Skipped) != 1 {
		t.Fatalf("expected 1 skipped entry, got %d", len(summary.Skipped))
	}
	if summary.Skipped[0].Reason != "resource skipped via -skip-create" {
		t.Fatalf("unexpected reason: %s", summary.Skipped[0].Reason)
	}
}
