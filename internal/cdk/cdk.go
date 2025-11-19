package cdk

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
)

// RunCDK2Pulumi runs the provided cdk2pulumi binary content on the given app path.
// It returns the path to the generated Pulumi.yaml (which is the output directory).
func RunCDK2Pulumi(binaryContent []byte, appPath string, stackName string) (string, error) {
	// Create a temporary file for the binary
	tmpFile, err := ioutil.TempFile("", "cdk2pulumi-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write the embedded binary to the temp file
	if _, err := tmpFile.Write(binaryContent); err != nil {
		return "", fmt.Errorf("failed to write binary to temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	// Make the binary executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}

	// Create a temporary directory for the output
	outputDir, err := ioutil.TempDir("", "cdk2pulumi-output-*")
	if err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Run the binary
	args := []string{appPath}
	if stackName != "" {
		args = append(args, "--stack", stackName)
	}
	cmd := exec.Command(tmpFile.Name(), args...)
	cmd.Dir = outputDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run cdk2pulumi: %w", err)
	}

	return outputDir, nil
}
