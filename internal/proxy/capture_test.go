package proxy

import (
	"sync"
	"testing"
)

func TestCaptureCollectorAppendAndCount(t *testing.T) {
	collector := NewCaptureCollector()
	collector.Append(Capture{Type: "aws:s3/bucket:Bucket", Name: "bucket", LogicalName: "Bucket", ID: "foo"})
	collector.Append(Capture{Type: "aws:s3/bucket:Bucket", Name: "bucket", LogicalName: "Bucket", ID: "foo"})

	if count := collector.Count(); count != 1 {
		t.Fatalf("expected 1 unique capture, got %d", count)
	}
	summary := collector.Summary()
	if summary.TotalIntercepts != 2 || summary.UniqueResources != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestCaptureCollectorConcurrent(t *testing.T) {
	collector := NewCaptureCollector()
	wg := sync.WaitGroup{}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			collector.Append(Capture{Type: "aws:lambda/function:Function", Name: "fn", LogicalName: "Fn", ID: "id"})
			collector.Append(Capture{Type: "aws:lambda/function:Function", Name: "fn", LogicalName: "Fn", ID: "id"})
			collector.Append(Capture{Type: "aws:sqs/queue:Queue", Name: "queue", LogicalName: "Queue", ID: "id"})
		}(i)
	}
	wg.Wait()

	results := collector.Results()
	if len(results) != 2 {
		t.Fatalf("expected 2 unique results, got %d", len(results))
	}
	summary := collector.Summary()
	if summary.UniqueResources != 2 {
		t.Fatalf("expected summary unique resources to be 2, got %d", summary.UniqueResources)
	}
	if summary.TotalIntercepts == 0 || summary.TotalIntercepts <= summary.UniqueResources {
		t.Fatalf("expected total intercepts > unique resources, summary=%+v", summary)
	}
}

func TestCaptureCollectorSkip(t *testing.T) {
	collector := NewCaptureCollector()
	collector.Skip(SkippedCapture{Type: "aws:iam/policy:Policy", LogicalName: "Inline", Reason: "unsupported"})
	summary := collector.Summary()
	if len(summary.Skipped) != 1 {
		t.Fatalf("expected 1 skipped entry, got %d", len(summary.Skipped))
	}
	if summary.Skipped[0].Reason != "unsupported" {
		t.Fatalf("unexpected skip reason: %+v", summary.Skipped[0])
	}
}
