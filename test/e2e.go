// End-to-end test that verifies both the server and CLI are working correctly.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/stiemannkj1/auto-update-example/common"
)

// runCommand executes an external command, captures its stdout and stderr,
// and panics if the command execution fails.
func runCommand(timeoutSecs int64, name string, args ...string) (stdout, stderr string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if err != nil {
		panic(fmt.Sprintf("Command failed: %s %v\nError:\n%v\nStdout:\n%s\nStderr:\n%s\n",
			name, args, err, stdout, stderr))
	}

	return stdout, stderr
}

func startCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Start()

	if err != nil {
		panic(fmt.Sprintf("Command failed: %s %v\nError:\n%v\nStdout:\n%s\nStderr:\n%s\n",
			name, args, err, stdoutBuf.String(), stderrBuf.String()))
	}

	return cmd
}

func copyFile(dst string, src string) error {

	srcFile, err := os.Open(src)

	if err != nil {
		return err
	}

	defer srcFile.Close()

	dstFile, err := os.Create(dst)

	if err != nil {
		return err
	}

	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)

	if err != nil {
		return err
	}

	if common.IsPosix() {

		srcInfo, err := srcFile.Stat()

		if err != nil {
			return err
		}

		err = os.Chmod(dst, srcInfo.Mode().Perm())

		if err != nil {
			return err
		}
	}

	return nil
}

func main() {

	// Clear out previous test binaries and data.
	_, err := os.Open("./test")

	if err != nil {
		panic(fmt.Sprintf("Error. e2e test must be run from project root:\n%v", err))
	}

	demoDir := "./test/demo"
	err = os.RemoveAll(demoDir)

	if err != nil {
		panic(fmt.Sprintf("Failed to clear out demo dir \"%s\":\n%v", demoDir, err))
	}

	err = os.MkdirAll(demoDir, 0b111111101)

	if err != nil {
		panic(fmt.Sprintf("Failed to create demo dir \"%s\":\n%v", demoDir, err))
	}

	// Build CLI v1.0.0
	var timeoutSecs int64 = 15
	var _ any
	_, _ = runCommand(
		timeoutSecs,
		"go",
		"build",
		"-ldflags",
		"-X 'main.Version=1.0.0' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'",
		"-o",
		"./test/demo/version/1.0.0/pokemon",
		"./pokemon",
	)

	// Copy 1.0.0 to the demo dir.
	src := "./test/demo/version/1.0.0/pokemon"
	dest := "./test/demo/pokemon"
	if err = copyFile(dest, src); err != nil {
		panic(fmt.Sprintf("Failed to copy \"%s\" to \"%s\":\n%v", src, dest, err))
	}

	// Build CLI v2.0.0
	_, _ = runCommand(
		timeoutSecs,
		"go",
		"build",
		"-ldflags",
		"-X 'main.Version=2.0.0' -X 'main.UpdateUrl=http://localhost:8080' -X 'main.AvailablePokemon=pikachu,raichu,charmander,charmeleon,squirtle,wartortle,bulbasaur,ivysaur'",
		"-o",
		"./test/demo/version/2.0.0/pokemon",
		"./pokemon",
	)

	// Build the server.
	_, _ = runCommand(timeoutSecs, "go", "build", "-o", "./test/demo/server", "./server")

	// Run the server.
	cmd := startCommand("./test/demo/server", "--settings", "test/server-properties.json")
	defer cmd.Process.Kill()

	// Wait for the server to start.
	err = nil
	start := time.Now().UnixMilli()

	for (time.Now().UnixMilli() - start) > timeoutSecs {
		var resp *http.Response
		resp, err = http.Get("http://localhost:8080/healthcheck")

		if err == nil && resp.StatusCode == 200 {
			break
		}

		// Otherwise retry.
	}

	if err != nil {
		panic(fmt.Sprintf("Failed to start server in %d seconds:\n%v", timeoutSecs, err))
	}

	// Attempt to run CLI v1.0.0 with a pokemon from v2.0.0. If the command
	// fails, the fail the test.
	stdout, stderr := runCommand(timeoutSecs, "test/demo/pokemon", "raichu")

	// If stdout doesn't show a greeting from raichu, fail the test.
	if !strings.Contains(strings.ToLower(stdout), "raichu") {
		panic(fmt.Sprintf("Test failed. \"%s\" not found in stdout.\nStdout:\n%s\nStderr:\n%s\n", "raichu", stdout, stderr))
	}

	// The server detected versions from the file system and exposed them via the API.
	// The CLI correctly updated and ran.
	fmt.Print("Test passed.\n")
}
