// CLI tool which shows greetings from various pokemon and automatically
// updates itself.
package main

import (
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/stiemannkj1/auto-update-example/common"
)

const POKEMON string = "pokemon"

// The env variable name that is used to determine if the process is the
// "updater" or the "updatee" (in other words, the actual CLI tool). If the
// value of this env variable is "TRUE", then the process will be the "updatee"
// tool and output greetings.
const POKEMON_CLI string = "POKEMON_CLI"

const SHORT_TIMEOUT_SECS = 1

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	} else {
		return " "
	}
}

// Signal to shutdown the "updatee" tool gracefully. The "updater" sends this
// value via stdin and the "updatee" should attempt to shut down immediately
// upon reading this value from stdin.
var shutdownSignal = []byte{1}

// Injected at build time:
var Version string
var UpdateUrl string

// TODO maybe change to embedded properties file
var AvailablePokemon string

// Prints CLI usage and available Pokemon.
func printUsage(version string, flags []common.CliFlag, availablePokemon []string) {
	fmt.Fprintf(os.Stderr, "Print a greeting from your favorite Pokemon.\nUsage: pokemon [(optional) Pokemon name]\n\n")

	for _, flag := range flags {
		fmt.Fprintf(os.Stderr, "%s, %s\n\t%s\n", flag.Name, flag.Short, flag.Description)
	}

	fmt.Fprintf(os.Stderr, "\nSupported Pokemon:\n\n")

	for _, pokemon := range availablePokemon {
		fmt.Fprintf(os.Stderr, "\t- %s\n", common.Capitalize(pokemon))
	}

	fmt.Fprintf(os.Stderr, "\nVersion: %s\n", version)
}

func main() {

	// Validate that the tool was built with the correct flags:
	if Version == "" {
		panic("Version must be specified in the build via `-ldflags \"-X 'main.Version=1.0.0'\"`")
	}

	if UpdateUrl == "" {
		panic("Default UpdateUrl must be specified via in the build via `-ldflags \"-X 'main.UpdateUrl=https://localhost:8080/'\"`")
	}

	if AvailablePokemon == "" {
		panic("At least one Pokemon must be specified in the build via `-ldflags \"-X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'\"`")
	}

	AvailablePokemon := strings.Split(AvailablePokemon, ",")

	for i := 0; i < len(AvailablePokemon); i += 1 {
		AvailablePokemon[i] = strings.ToLower(strings.TrimSpace(AvailablePokemon[i]))

		if AvailablePokemon[i] == "" {
			panic(fmt.Sprintf("AvailablePokemon at %d was blank.", i))
		}
	}

	if len(AvailablePokemon) == 0 {
		panic("No AvailablePokemon found.")
	}

	// Get the current executable, its metadata, and its parent:
	exe, err := os.Executable()

	if err != nil {
		panic(fmt.Sprintf("Error getting current executable dir:\n%v", err))
	}

	exe, err = filepath.Abs(exe)

	if err != nil {
		panic(fmt.Sprintf("Error getting current executable dir:\n%v", err))
	}

	exeDir := filepath.Dir(exe)
	exeStat, err := os.Stat(exe)

	if err != nil {
		panic(fmt.Sprintf("Error getting current executable permissions:\n%v", err))
	}

	exePermissions := exeStat.Mode().Perm()

	helpFlag := common.CliFlag{
		Name:        "--help",
		Short:       "-h",
		Description: "Print this help message",
	}
	versionFlag := common.CliFlag{
		Name:        "--version",
		Short:       "-v",
		Description: "Print the version of this cli tool",
	}
	updateUrlFlag := common.CliFlag{
		Name:        "--update-url",
		Short:       "-u",
		Description: fmt.Sprintf("(optional) The url to obtain updates from (https is required). Defaults to %s", UpdateUrl),
	}

	var daemonIntervalSecs uint64 = 1
	daemonFlag := common.CliFlag{
		Name:        "--daemon",
		Short:       "-d",
		Description: fmt.Sprintf("(optional) Run this executable in daemon mode outputting a Pokemon greeting on an interval. Configure the interval in seconds by specifying an optional positive integer. Defaults to %d second(s) if interval is unspecified", daemonIntervalSecs),
	}

	var updateCheckIntervalSecs uint64 = 15
	updateIntervalFlag := common.CliFlag{
		Name:        "--update-check-interval",
		Short:       "-u",
		Description: fmt.Sprintf("(optional) Interval to check for updates when running in daemon mode. Defaults to %d second(s)", updateCheckIntervalSecs),
	}

	flags := []common.CliFlag{helpFlag, versionFlag, updateUrlFlag, daemonFlag, updateIntervalFlag}

	var pokemon string
	args := os.Args

	daemonRun := false

	// Avoid using `flag` package here since we need to customize our arg parsing code.
	// Parse CLI args:``
	for i := 1; i < len(args); i += 1 {
		switch args[i] {
		case helpFlag.Name, helpFlag.Short:
			printUsage(Version, flags, AvailablePokemon)
			return
		case versionFlag.Name, versionFlag.Short:
			fmt.Fprintf(os.Stderr, "%s\n", Version)
			return
		case updateUrlFlag.Name, updateUrlFlag.Short:
			if i+1 < len(args) {
				i += 1
				UpdateUrl = args[i]
			} else {
				fmt.Fprint(os.Stderr, "No value provided for --update-url\n")
				printUsage(Version, flags, AvailablePokemon)
				os.Exit(64)
			}

			// Require https for user supplied URLs.
			// Allow non-https for the default URL for the sake of testing.
			if !strings.HasPrefix(UpdateUrl, "https:") {
				fmt.Fprintf(os.Stderr, "--update-url must use https: %s\n", UpdateUrl)
				printUsage(Version, flags, AvailablePokemon)
				os.Exit(64)
			}
		case daemonFlag.Name, daemonFlag.Short:
			daemonRun = true

			var err error

			if i+1 < len(args) {
				daemonIntervalSecs, err = strconv.ParseUint(args[i+1], 10, 16)
			}

			if err != nil {
				// Ignore, this might be a Pokemon or another argument.
			} else {
				i += 1
			}
		case updateIntervalFlag.Name, updateIntervalFlag.Short:

			var err error

			hasValue := i+1 < len(args)

			if hasValue {
				i += 1
				daemonIntervalSecs, err = strconv.ParseUint(args[i], 10, 16)
			}

			if !hasValue || err != nil {
				fmt.Fprintf(os.Stderr, "%s requires a positive integer value.\n", updateIntervalFlag.Name)
				printUsage(Version, flags, AvailablePokemon)
				os.Exit(64)
			}
		default:
			if len(args[i]) == 0 || args[i][0] == '-' {
				fmt.Fprintf(os.Stderr, "Invalid flag: \"%s\"\n", args[i])
				printUsage(Version, flags, AvailablePokemon)
				os.Exit(64)
			} else {
				pokemon = strings.ToLower(args[i])
			}
		}
	}

	if strings.ToUpper(os.Getenv(POKEMON_CLI)) != "TRUE" {

		// If the updater completely fails for some bizarre reason, we fall
		// back to simply running the command directly without any update
		// functionality. Barring errors, the update loop method should not
		// exit.
		err = updateLoop(exe, exeDir, exePermissions, daemonRun, Version, UpdateUrl, updateCheckIntervalSecs)

		if err == nil {
			return
		}

		fmt.Fprintf(os.Stderr, "Failed to use updateable version:\n%v\nFalling back to non-updatable execution.\n", err)
	}

	// Update before validating pokemon in case the update supports a new
	// pokemon.
	randomPokemon := false

	if pokemon == "" {
		randomPokemon = true
	} else if !slices.Contains(AvailablePokemon, pokemon) {
		fmt.Fprintf(os.Stderr, "%s is not a supported Pokemon.\n", common.Capitalize(pokemon))
		os.Exit(64)
	}

	var stdin []byte = make([]byte, 0, 1)

	// Print greeting:
	for {

		// Listen for shutdown request and exit if you recieve it.
		if read, err := os.Stdin.Read(stdin); err != nil && read > 0 && stdin[0] > 0 {
			os.Exit(0)
		}

		if randomPokemon {
			pokemon = AvailablePokemon[rand.Intn(len(AvailablePokemon))]
		}

		fmt.Printf("%s says, \"Hi!\".\n", common.Capitalize(pokemon))

		if !daemonRun {
			return
		} else {
			time.Sleep(time.Duration(daemonIntervalSecs) * time.Second)
		}
	}
}

type Cmd struct {
	Version string
	Path    string
	Cmd     *exec.Cmd
	Stdin   io.WriteCloser
}

func kill(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Process.Release()
	}
}

// Infinite loop that updates the CLI by:
// 1. Finding the latest version.
// 2. Downloading and verifying the latest version.
// 3. Shutting down the previous version.
// 4. Starting the new version.
// This function will also attempt to fall back to previous working versions if
// there are problems.
func updateLoop(exe string, exeDir string, exePermissions fs.FileMode, isDaemon bool, initialVersion string, updateUrl string, updateCheckIntervalSecs uint64) error {

	// Propagate this value to child processes.
	err := os.Setenv(POKEMON_CLI, "TRUE")

	if err != nil {
		return fmt.Errorf("update failed to set %s", POKEMON_CLI)
	}

	var prevCmd Cmd
	var currentCmd Cmd
	updateFilePath := ""

	first := true

	for {

		// If this is a non-daemon process, it should execute and exit immediately.
		if !isDaemon && currentCmd.Cmd != nil {
			err = currentCmd.Cmd.Wait()

			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to wait for child process:\n%v\n", err)
				kill(currentCmd.Cmd)
				os.Exit(1)
			}

			time.Sleep(time.Duration(SHORT_TIMEOUT_SECS) * time.Second)
			os.Exit(currentCmd.Cmd.ProcessState.ExitCode())
		}

		if first {
			first = false
		} else {
			time.Sleep(time.Duration(updateCheckIntervalSecs) * time.Second)
		}

		fmt.Printf("Checking for updates...\n")

		// TODO configure limits on versions to update.
		version, err := getLatestVersion(updateUrl)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed determine versions available for updates:\n%v\n", err)
		} else if currentCmd.Version == version {
			fmt.Printf("%s is the already latest version.\n", version)
			continue
		}

		// TODO handle name collisions.
		updateFilePath, err = downloadUpdateVersion(exeDir, updateUrl, version, exePermissions)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to download update file:\n%v\n", err)
		} else {
			var newCmd Cmd
			newCmd, err = upgradeChildProcess(currentCmd, updateFilePath, version)

			if err == nil {
				prevCmd = currentCmd
				currentCmd = newCmd
				fmt.Printf("Successfully updated to version %s.\n", version)
				continue
			}

			fmt.Fprintf(os.Stderr, "Failed to start process \"%s\":\n%v\n", updateFilePath, err)
		}

		// Attempt to fall back to the last known working version.
		if prevCmd.Path != "" && prevCmd.Path != updateFilePath {
			fmt.Fprintf(os.Stderr, "Falling back to \"%s\".\n", prevCmd.Version)

			currentCmd, err = upgradeChildProcess(currentCmd, prevCmd.Path, prevCmd.Version)

			if err == nil {
				fmt.Printf("Successfully reverted to \"%s\".", prevCmd.Version)
				continue
			}

			fmt.Fprintf(os.Stderr, "Failed to start process \"%s\":\n%v\n", prevCmd.Path, err)
		}

		// Fall back to the current version since we at least know it was installed.
		fmt.Fprintf(os.Stderr, "Falling back to \"%s\".\n", initialVersion)

		currentCmd, err = upgradeChildProcess(currentCmd, exe, initialVersion)

		if err != nil {
			return fmt.Errorf("failed to use default version")
		}
	}
}

// Gets the latest available version from the server.
func getLatestVersion(updateUrl string) (string, error) {

	resp, err := http.Get(fmt.Sprintf("%s/v1.0/versions/%s", updateUrl, POKEMON))

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	var versions common.Versions

	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil || len(versions.All) == 0 {
		return "", err
	}

	return versions.All[len(versions.All)-1], nil
}

// Downloads the specified version of the tool if it doesn't already exist on
// the filesystem.
func downloadUpdateVersion(exeDir string, updateUrl string, version string, permissions fs.FileMode) (string, error) {

	if version == "" {
		return "", fmt.Errorf("version was empty")
	}

	// TODO maybe handle file name collisions with the temp files though
	// they're extremely unlikely.
	updateFilePath := filepath.Join(exeDir, fmt.Sprintf("%s-%s%s", POKEMON, version, exeSuffix()))
	updateFile, err := os.Open(updateFilePath)
	alreadyExists := err == nil

	if alreadyExists {
		defer updateFile.Close()
	}

	resp, err := http.Get(fmt.Sprintf("%s/v1.0/downloads/%s?version=%s", updateUrl, POKEMON, version))

	if err != nil {
		return "", err
	}

	// Validate the file if it has already been downloaded.
	if alreadyExists {

		sha512, err := common.Sha512Hash(updateFile)

		if err != nil {
			return "", err
		}

		expectedSha512 := resp.Header.Get(common.Sha512Name)

		if expectedSha512 != sha512 {
			return "", common.NewSha512Error(updateFilePath, expectedSha512, sha512)
		}

		// Update file already exists.
		return updateFilePath, nil
	}

	defer resp.Body.Close()

	// Download to a temp file to attempt an atomic move on Unix systems.
	// The temp file should be created in the same dir that the target file
	// exists in. This prevents the file from being moved across
	// filesystems.
	updateFileTempPath := filepath.Join(exeDir, fmt.Sprintf(".%s-%s.%d.tmp", POKEMON, version, time.Now().UnixNano()))
	if updateFile, err = os.OpenFile(updateFileTempPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, permissions); err != nil {
		return "", err
	}

	defer os.Remove(updateFileTempPath)

	defer updateFile.Close()

	hasher := sha512.New()

	if _, err = io.Copy(io.MultiWriter(hasher, updateFile), resp.Body); err != nil {
		return "", err
	}

	sha512 := common.ToHexHash(&hasher)
	expectedSha512 := resp.Header.Get(common.Sha512Name)

	if expectedSha512 != sha512 {
		return "", common.NewSha512Error(updateFilePath, expectedSha512, sha512)
	}

	if err = updateFile.Sync(); err != nil {
		return "", err
	}

	// Close the file to avoid locking it on Windows and failing the rename
	// below.
	if err = updateFile.Close(); err != nil {
		return "", err
	}

	// Attempt atomic move.
	if err = os.Rename(updateFileTempPath, updateFilePath); err != nil {
		return "", err
	}

	return updateFilePath, nil
}

// Stops the previous child process and starts the current one.
func upgradeChildProcess(previousChild Cmd, updateFilePath string, version string) (Cmd, error) {

	if previousChild != (Cmd{}) {

		// Attempt to gracefully shutdown the previous process.
		var err error

		for range 3 {

			var wrote int
			wrote, err = previousChild.Stdin.Write(shutdownSignal)

			if err != nil {
				break
			} else if wrote > 0 {
				break
			}

			// Retry when no bytes written.
		}

		if err == nil {
			time.Sleep(time.Duration(SHORT_TIMEOUT_SECS) * time.Second)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to shutdown process gracefully:\n%v\n", err)
		}

		fmt.Fprintf(os.Stderr, "Shutting down %s.\n", previousChild.Version)

		// If the previous process hasn't already shut down, force it to shut
		// down.
		kill(previousChild.Cmd)
	}

	// Create the new process.
	cmd := exec.Command(updateFilePath, os.Args[1:]...)

	stdin, err := cmd.StdinPipe()

	if err != nil {
		return Cmd{}, err
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()

	if err != nil {
		return Cmd{}, err
	}

	return Cmd{
		Version: version,
		Path:    updateFilePath,
		Cmd:     cmd,
		Stdin:   stdin,
	}, nil
}
