package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestFriendlyHandlerFormatsNumericAttrs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := New(&buf, false, "component", "cdk-importer")
	logger.Info("Run complete", "status", "success", "resourcesImported", 10, "resourcesFailedToImport", 0)

	line := buf.String()
	if strings.Contains(line, "resourcesImported='") {
		t.Fatalf("resourcesImported should not be quoted: %s", line)
	}
	if !strings.Contains(line, "resourcesImported=10") {
		t.Fatalf("resourcesImported count missing: %s", line)
	}
	if !strings.Contains(line, "resourcesFailedToImport=0") {
		t.Fatalf("resourcesFailedToImport count missing: %s", line)
	}
	if !strings.Contains(line, "status=\"success\"") {
		t.Fatalf("status should remain quoted: %s", line)
	}
}

func TestFriendlyHandlerKeepsMultilineStrings(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := New(&buf, false, "component", "cdk-importer")
	logger.Info("Pulumi errors", "details", "Diagnostics:\n  resource: failed\n  another: failed")

	output := buf.String()
	if !strings.Contains(output, "Diagnostics:\n  resource: failed\n  another: failed") {
		t.Fatalf("multiline details should not be escaped: %s", output)
	}
}
