package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/changesummary"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"
)

func runCmd(t *testing.T, workspace auto.Workspace, commandPath string, args []string) error {
	env := os.Environ()
	for k, v := range workspace.GetEnvVars() {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	command := strings.Join(args, " ")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, commandPath, args...)
	defer cancel()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.WaitDelay = time.Second * 1
	cmd.Env = env
	cmd.Dir = workspace.WorkDir()

	runerr := cmd.Run()
	if runerr != nil {
		t.Logf("Invoke Start '%v' failed: %s\n", command, runerr)
		if runerr == exec.ErrWaitDelay {
			return nil
		}
	}
	return runerr
}

func runCdkCommand(t *testing.T, workspace auto.Workspace, args []string) error {
	return runCmd(t, workspace, "node_modules/.bin/cdk", args)
}

func skipIfShort(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping in testing.Short() mode, assuming this is a CI run without credentials")
	}

}

func runImportCommand(t *testing.T, workspace auto.Workspace, stackName string) error {
	binPath, err := filepath.Abs("../bin")
	if err != nil {
		t.Fatal(err)
	}
	commandPath := filepath.Join(binPath, "pulumi-tool-cdk-importer")
	args := []string{"-stack", stackName}
	return runCmd(t, workspace, commandPath, args)
}

func TestImport(t *testing.T) {
	skipIfShort(t)
	sourceDir := filepath.Join(getCwd(t), "cdk-test")
	test := newPulumiTest(t, sourceDir)
	suffix := getSuffix()
	cdkStackName := fmt.Sprintf("import-test-%s", suffix)

	tmpDir := test.CurrentStack().Workspace().WorkDir()
	test.CurrentStack().Workspace().SetEnvVar("CDK_APP_ID_SUFFIX", suffix)

	defer func() {
		test.Destroy(t)
		runCdkCommand(t, test.CurrentStack().Workspace(), []string{"destroy", "--require-approval", "never", "--all", "--force"})
	}()

	t.Logf("Working directory: %s", tmpDir)
	// deploy cdk app
	err := runCdkCommand(t, test.CurrentStack().Workspace(), []string{"deploy", "--require-approval", "never", "--all"})
	require.NoError(t, err)

	t.Log("Importing resources")

	// import cdk app
	err = runImportCommand(t, test.CurrentStack().Workspace(), cdkStackName)
	require.NoError(t, err)

	t.Log("Import complete")

	previewResult := test.Preview(t, optpreview.Diff())
	t.Logf("Stderr=%s", previewResult.StdErr)
	t.Logf("Stdout=%s", previewResult.StdOut)
	summary := changesummary.ChangeSummary(previewResult.ChangeSummary)
	creates := summary.WhereOpEquals(apitype.OpCreate)
	assert.Equal(t, 0, len(*creates), "Expected no creates")
}

func getEnvRegion(t *testing.T) string {
	envRegion := os.Getenv("AWS_REGION")
	if envRegion == "" {
		t.Skipf("Skipping test due to missing AWS_REGION environment variable")
	}

	return envRegion
}

func getSuffix() string {
	prefix := os.Getenv("GITHUB_SHA")
	if prefix == "" {
		prefix = strconv.Itoa(rand.Intn(10000))
	}
	if len(prefix) > 5 {
		prefix = prefix[:5]
	}
	// has to start with a letter
	return fmt.Sprintf("a%s", prefix)
}

func getCwd(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.FailNow()
	}

	return cwd
}

func newPulumiTest(t *testing.T, source string) *pulumitest.PulumiTest {
	envRegion := getEnvRegion(t)
	test := pulumitest.NewPulumiTest(t, source)
	test.SetConfig(t, "aws:region", envRegion)
	test.SetConfig(t, "aws-native:region", envRegion)
	return test
}
