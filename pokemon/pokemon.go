// CLI tool which shows greetings from various pokemon.
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
	"syscall"
	"time"

	"github.com/stiemannkj1/auto-update-example/common"
)

const Pokemon string = "pokemon"
const PokemonCliUpdatedName string = "POKEMON_CLI_UPDATED"

// Injected at build time:
var Version string
var UpdateUrl string
var AvailablePokemon string

// Prints CLI usage and available Pokemon.
func printUsage(version string, flags []common.CliFlag, availablePokemon []string) {
	fmt.Fprintf(os.Stderr, "Print a greeting from your favorite Pokemon.\nUsage: pokemon [(optional) Pokemon name]\n\n")

	for _, flag := range flags {
		fmt.Fprintf(os.Stderr, "%s, %s\n\t%s\n", flag.Name, flag.Short, flag.Description)
	}

	fmt.Fprintf(os.Stderr, "\nSupported Pokemon:\n\n")

	for _, pokemon := range availablePokemon {
		fmt.Fprintf(os.Stderr, "\t- %s%s\n", strings.ToUpper(pokemon[0:1]), pokemon[1:])
	}

	fmt.Fprintf(os.Stderr, "\nVersion: %s\n", version)
}

// Copies old CLI args to a new array and changes the 0th arg to point to a new
// executable path.
func newArgs(oldArgs []string, newExe string) []string {

	if len(oldArgs) == 0 {
		return []string{}
	}

	newArgs := make([]string, len(oldArgs))
	copied := copy(newArgs, oldArgs)

	if copied != len(oldArgs) {
		panic(fmt.Sprintf("Expected %d args to be copied, but instead %d were copied.", len(oldArgs), copied))
	}

	newArgs[0] = newExe
	return newArgs
}

// Gets the latest available version from the server.
func getLatestVersion(updateUrl string) (string, error) {

	resp, err := http.Get(fmt.Sprintf("%s/versions/%s", updateUrl, Pokemon))

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

	// TODO handle file name collisions.
	updateFilePath := filepath.Join(exeDir, fmt.Sprintf("%s-%s", Pokemon, version))
	updateFile, err := os.Open(updateFilePath)
	alreadyExists := err == nil

	if alreadyExists {
		defer updateFile.Close()
	}

	resp, err := http.Get(fmt.Sprintf("%s/downloads/%s?version=%s", updateUrl, Pokemon, version))

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
	updateFileTempPath := filepath.Join(exeDir, fmt.Sprintf(".%s-%s.%d.tmp", Pokemon, version, time.Now().UnixNano()))
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

	// Attempt atomic move.
	if err = os.Rename(updateFileTempPath, updateFilePath); err != nil {
		return "", err
	}

	return updateFilePath, nil
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
	}

	// Get the current executable, its metadata, and its parent:
	exe, err := os.Executable()

	if err != nil {
		panic(fmt.Sprintf("Error getting current executable dir: %v", err))
	}

	exe, err = filepath.Abs(exe)

	if err != nil {
		panic(fmt.Sprintf("Error getting current executable dir: %v", err))
	}

	exeDir := filepath.Dir(exe)
	exeStat, err := os.Stat(exe)

	if err != nil {
		panic(fmt.Sprintf("Error getting current executable permissions: %v", err))
	}

	exePermissions := exeStat.Mode().Perm()

	isPosix := false
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "netbsd", "openbsd", "solaris":
		isPosix = true
	}

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

	randomPokemon := false

	if pokemon == "" {
		randomPokemon = true
	} else if !slices.Contains(AvailablePokemon, pokemon) {
		fmt.Fprintf(os.Stderr, "%s is not a supported Pokemon.\n", pokemon)
		return
	}

	// Updates the CLI by:
	// 1. Downloading and verifying the latest version.
	// 2. Starting the new version.
	// 3. Shutting down the current version.
	update := func() {

		fmt.Printf("Checking for updates...\n")

		// TODO configure limits on versions to update.
		version, err := getLatestVersion(UpdateUrl)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed determine versions available for updates: %v\n", err)
			return
		}

		if version == Version {
			fmt.Printf("%s is the already latest version.\n", version)
			return
		}

		updateFilePath, err := downloadUpdateVersion(exeDir, UpdateUrl, version, exePermissions)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to download update file: %v\n", err)
			return
		}

		err = os.Setenv(PokemonCliUpdatedName, "TRUE")

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to set up env for update: %v\n", err)
			return
		}

		if isPosix {

			// On Posix, reuse the existing process via exec for a seamless upgrade
			// that keeps the existing PID.
			//
			// If the CLI ever needs to clean up resources, we may need to force
			// forking another process instead.
			err = syscall.Exec(updateFilePath, newArgs(os.Args, updateFilePath), os.Environ())

			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to start updated version: %v\n", err)
			} else {
				// This line should never be hit as exec exits abruptly.
				os.Exit(0)
			}
		}

		cmd := exec.Command(updateFilePath, newArgs(os.Args, updateFilePath)...)
		err = cmd.Start()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start updated version: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Started updated version: %s. Shutting down: %s.\n", version, Version)
			os.Exit(0)
		}
	}

	if os.Getenv(PokemonCliUpdatedName) == "TRUE" {
		// Skip update if we already know we just updated.
		_ = os.Unsetenv(PokemonCliUpdatedName)
		fmt.Printf("Successfully updated to %s\n", Version)
	} else {
		update()
	}

	// Background thread to update versions. This thread may be killed at any
	// time, so don't expect any defer calls the complete. This should not be
	// used for writing external data to the filesystem.
	go func() {
		for {
			time.Sleep(time.Duration(updateCheckIntervalSecs) * time.Second)

			update()
		}
	}()

	// Print greeting:
	for {
		if randomPokemon {
			pokemon = AvailablePokemon[rand.Intn(len(AvailablePokemon))]
		}

		fmt.Printf("%s%s says, \"Hi!\".\n", strings.ToUpper(pokemon[0:1]), pokemon[1:])

		if !daemonRun {
			return
		} else {
			time.Sleep(time.Duration(daemonIntervalSecs) * time.Second)
		}
	}
}
