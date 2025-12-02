package proxy

import (
	"log"
	"os"
	"path/filepath"
	"testing"

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
	logger := log.New(os.Stdout, "[test] ", log.Ltime)

	collector := NewCaptureCollector()
	collector.Append(Capture{
		Type:        "aws:s3/bucket:Bucket",
		Name:        "test-bucket",
		LogicalName: "TestBucket",
		ID:          "bucket-123",
		Properties:  []string{"tags"},
	})

	// Test with empty deployment (partial result)
	err := finalizeCapture(logger, collector, importPath, apitype.UntypedDeployment{}, true, nil)
	require.NoError(t, err)

	// Verify file was written
	_, err = os.Stat(importPath)
	require.NoError(t, err, "import file should exist even with partial results")

	// Test with complete deployment
	importPath2 := filepath.Join(tmpDir, "import2.json")
	err = finalizeCapture(logger, collector, importPath2, apitype.UntypedDeployment{}, false, nil)
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
	logger := log.New(&testWriter{output: &logOutput}, "[test] ", 0)

	collector := NewCaptureCollector()
	err := finalizeCapture(logger, collector, importPath, apitype.UntypedDeployment{}, true, nil)
	require.NoError(t, err)

	// Verify partial status is logged
	assert.Contains(t, string(logOutput), "partial", "should log partial status")
}

type testWriter struct {
	output *[]byte
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	*w.output = append(*w.output, p...)
	return len(p), nil
}
