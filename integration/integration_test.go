package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/changesummary"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"
)

type cmdOutput struct {
	t *testing.T
}

func (c cmdOutput) Write(p []byte) (n int, err error) {
	c.t.Log(string(p))
	return len(p), nil
}

func runCmd(t *testing.T, workspace auto.Workspace, commandPath string, args []string) error {
	env := os.Environ()
	for k, v := range workspace.GetEnvVars() {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	cmd := exec.Command(commandPath, args...)
	command := strings.Join(args, " ")
	cmd.Env = env
	cmd.Dir = workspace.WorkDir()
	cmd.Stdout = cmdOutput{t}
	cmd.Stderr = cmdOutput{t}

	runerr := cmd.Run()
	if runerr != nil {
		t.Logf("Invoke '%v' failed: %s\n", command, runerr.Error())
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
	test := pulumitest.NewPulumiTest(t, sourceDir)

	tmpDir := test.CurrentStack().Workspace().WorkDir()

	defer func() {
		runCdkCommand(t, test.CurrentStack().Workspace(), []string{"destroy", "--require-approval", "never", "--all", "--force"})
		test.Destroy(t)
	}()

	t.Logf("Working directory: %s", tmpDir)
	// deploy cdk app
	err := runCdkCommand(t, test.CurrentStack().Workspace(), []string{"deploy", "--require-approval", "never", "--all"})
	require.NoError(t, err)

	t.Log("Importing resources")

	// import cdk app
	err = runImportCommand(t, test.CurrentStack().Workspace(), "import-test")
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

func getBaseOptions(t *testing.T) integration.ProgramTestOptions {
	envRegion := getEnvRegion(t)
	suffix := getSuffix()
	return integration.ProgramTestOptions{
		Config: map[string]string{
			"aws:region":        envRegion,
			"aws-native:region": envRegion,
		},
		Env: []string{"CDK_APP_ID_SUFFIX=" + suffix},
		// some flakiness in some resource creation
		// @see https://github.com/pulumi/pulumi-aws-native/issues/1714
		RetryFailedSteps:     true,
		ExpectRefreshChanges: true,
		SkipRefresh:          true,
		Quick:                true,
	}
}