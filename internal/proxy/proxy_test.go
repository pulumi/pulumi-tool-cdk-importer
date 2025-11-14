package proxy

import "testing"

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

	if got := deriveCaptureStackName("my-stack", ""); got != "capture-my-stack" {
		t.Fatalf("unexpected derived stack: %s", got)
	}

	if got := deriveCaptureStackName("ignored", "/tmp/state.json"); got != "state" {
		t.Fatalf("expected file-derived stack name, got %s", got)
	}

	if got := deriveCaptureStackName("", ""); got != "" {
		t.Fatalf("expected empty when no inputs, got %s", got)
	}
}
