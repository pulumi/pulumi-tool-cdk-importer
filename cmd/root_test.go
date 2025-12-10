package cmd

import "testing"

func TestRootCommandSilencesCobraErrors(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if !cmd.SilenceErrors {
		t.Fatalf("expected SilenceErrors to be true to avoid duplicate error output")
	}
	if !cmd.SilenceUsage {
		t.Fatalf("expected SilenceUsage to be true to avoid printing usage on runtime errors")
	}
}
