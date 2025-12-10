package cmd

import "testing"

func TestFormatCLIErrorTrimsAutomationNoise(t *testing.T) {
	t.Parallel()

	err := fakeError("failed to run update: exit status 255\ncode: 255\nstdout: Updating (chall-dev)\n\nstderr: some usage text")
	got := formatCLIError(err)
	if got != "failed to run update: exit status 255" {
		t.Fatalf("unexpected formatted error: %q", got)
	}
}

type fakeError string

func (f fakeError) Error() string {
	return string(f)
}
