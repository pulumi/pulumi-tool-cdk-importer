package proxy

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/imports"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeStackComponent(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"MyStack":      "MyStack",
		"stack with *": "stack-with",
		"":             "",
		"@@@":          "",
		"stack.name":   "stack.name",
		"stack/name":   "stack-name",
		"stack:name":   "stack-name",
	}

	for input, expected := range cases {
		t.Run(input, func(t *testing.T) {
			if got := sanitizeStackComponent(input); got != expected {
				t.Fatalf("sanitizeStackComponent(%q) = %q, want %q", input, got, expected)
			}
		})
	}
}

func TestDeriveCaptureStackName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		stackRef  string
		stackFile string
		want      string
	}{
		{
			name:      "stack ref only",
			stackRef:  "my-stack",
			stackFile: "",
			want:      "capture-my-stack",
		},
		{
			name:      "stack file only",
			stackRef:  "ignored",
			stackFile: "path/to/my-stack.json",
			want:      "my-stack",
		},
		{
			name:      "both provided prefers file",
			stackRef:  "ignored",
			stackFile: "path/to/my-stack.json",
			want:      "my-stack",
		},
		{
			name:      "sanitization",
			stackRef:  "My/Stack!",
			stackFile: "",
			want:      "capture-My-Stack",
		},
		{
			name:      "empty",
			stackRef:  "",
			stackFile: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stacks []string
			if tt.stackRef != "" {
				stacks = []string{tt.stackRef}
			}
			got := deriveCaptureStackName(stacks, tt.stackFile)
			if got != tt.want {
				t.Errorf("deriveCaptureStackName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFinalizeCaptureWithPartialResults(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	importPath := filepath.Join(tmpDir, "import.json")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	collector := NewCaptureCollector()
	collector.Append(Capture{
		Type:        "aws:s3/bucket:Bucket",
		Name:        "test-bucket",
		LogicalName: "TestBucket",
		ID:          "bucket-123",
		Properties:  []string{"tags"},
	})

	// Test with empty deployment (partial result)
	err := finalizeCapture(logger, collector, importPath, apitype.UntypedDeployment{}, true, nil, false)
	require.NoError(t, err)

	// Verify file was written
	_, err = os.Stat(importPath)
	require.NoError(t, err, "import file should exist even with partial results")

	// Test with complete deployment
	importPath2 := filepath.Join(tmpDir, "import2.json")
	err = finalizeCapture(logger, collector, importPath2, apitype.UntypedDeployment{}, false, nil, false)
	require.NoError(t, err)

	// Verify file was written
	_, err = os.Stat(importPath2)
	require.NoError(t, err, "import file should exist with complete results")
}

func TestFinalizeCaptureLogsPartialStatus(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	importPath := filepath.Join(tmpDir, "import.json")

	// Capture log output
	var logOutput []byte
	logger := slog.New(slog.NewTextHandler(&testWriter{output: &logOutput}, &slog.HandlerOptions{Level: slog.LevelInfo}))

	collector := NewCaptureCollector()
	err := finalizeCapture(logger, collector, importPath, apitype.UntypedDeployment{}, true, nil, false)
	require.NoError(t, err)

	// Verify partial status is logged
	assert.Contains(t, string(logOutput), "partial", "should log partial status")
}

func TestFinalizeCaptureFiltersPlaceholders(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	importPath := filepath.Join(tmpDir, "import.json")
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))

	collector := NewCaptureCollector()
	collector.Append(Capture{
		Type: "aws:s3/bucket:Bucket",
		Name: "withId",
		ID:   "bucket-123",
	})
	collector.Append(Capture{
		Type: "aws:s3/bucket:Bucket",
		Name: "missingId",
		ID:   "<PLACEHOLDER>",
	})

	err := finalizeCapture(logger, collector, importPath, apitype.UntypedDeployment{}, false, nil, true)
	require.NoError(t, err)

	file, err := imports.ReadFile(importPath)
	require.NoError(t, err)

	if assert.Len(t, file.Resources, 1) {
		assert.Equal(t, "<PLACEHOLDER>", file.Resources[0].ID)
		assert.Equal(t, "missingId", file.Resources[0].Name)
	}
}

func TestUpEventTrackerCountsCreatesAndFailures(t *testing.T) {
	t.Parallel()

	tracker := newUpEventTracker()
	tracker.handle(events.EngineEvent{EngineEvent: apitype.EngineEvent{
		ResourcePreEvent: &apitype.ResourcePreEvent{
			Metadata: apitype.StepEventMetadata{URN: "urn:pulumi:stack::project::pkg:mod:Type::name", Op: apitype.OpCreate},
		},
	}})
	tracker.handle(events.EngineEvent{EngineEvent: apitype.EngineEvent{
		DiagnosticEvent: &apitype.DiagnosticEvent{URN: "urn:pulumi:stack::project::pkg:mod:Type::name", Severity: "error", Message: "boom"},
	}})
	tracker.handle(events.EngineEvent{EngineEvent: apitype.EngineEvent{
		ResOpFailedEvent: &apitype.ResOpFailedEvent{
			Metadata: apitype.StepEventMetadata{URN: "urn:pulumi:stack::project::pkg:mod:Type::name", Op: apitype.OpCreate},
		},
	}})
	tracker.handle(events.EngineEvent{EngineEvent: apitype.EngineEvent{
		ResourcePreEvent: &apitype.ResourcePreEvent{
			Metadata: apitype.StepEventMetadata{URN: "urn:pulumi:stack::project::pkg:mod:Type::other", Op: apitype.OpCreate},
		},
	}})
	tracker.handle(events.EngineEvent{EngineEvent: apitype.EngineEvent{
		ResOutputsEvent: &apitype.ResOutputsEvent{
			Metadata: apitype.StepEventMetadata{URN: "urn:pulumi:stack::project::pkg:mod:Type::other", Op: apitype.OpCreate},
		},
	}})

	assert.Equal(t, 2, tracker.totalResources(), "should track registered resources")
	assert.Equal(t, 1, tracker.created(), "should count successful creates")
	assert.Equal(t, 1, tracker.failedCreates(), "should count failed creates")
	assert.Equal(t, "urn:pulumi:stack::project::pkg:mod:Type::name: boom", tracker.failureSummary())
}

func TestUpEventTrackerUsesGeneralDiagnostics(t *testing.T) {
	t.Parallel()

	tracker := newUpEventTracker()
	tracker.handle(events.EngineEvent{EngineEvent: apitype.EngineEvent{
		DiagnosticEvent: &apitype.DiagnosticEvent{Severity: "error", Message: "stack failed"},
	}})
	tracker.handle(events.EngineEvent{EngineEvent: apitype.EngineEvent{
		ResOpFailedEvent: &apitype.ResOpFailedEvent{
			Metadata: apitype.StepEventMetadata{URN: "urn:pulumi:stack::project::pkg:mod:Type::name", Op: apitype.OpCreate},
		},
	}})

	assert.Equal(t, 1, tracker.failedCreates(), "should count failure even without URN-specific diagnostic")
	assert.Equal(t, "stack failed", tracker.failureSummary(), "should fall back to general diagnostics")
}

func TestUpEventTrackerSummarizesDiagnosticsWithoutFailures(t *testing.T) {
	t.Parallel()

	tracker := newUpEventTracker()
	tracker.handle(events.EngineEvent{EngineEvent: apitype.EngineEvent{
		DiagnosticEvent: &apitype.DiagnosticEvent{URN: "urn:pulumi:stack::project::pkg:mod:Type::name", Severity: "error", Message: "boom"},
	}})
	tracker.handle(events.EngineEvent{EngineEvent: apitype.EngineEvent{
		DiagnosticEvent: &apitype.DiagnosticEvent{Severity: "error", Message: "stack failed before starting"},
	}})

	assert.Equal(t, "urn:pulumi:stack::project::pkg:mod:Type::name: boom\n\nstack failed before starting", tracker.failureSummary())
}

type testWriter struct {
	output *[]byte
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	*w.output = append(*w.output, p...)
	return len(p), nil
}
