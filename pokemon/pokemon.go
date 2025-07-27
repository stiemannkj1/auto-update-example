package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
)

type CliFlag struct {
	Name        string
	Short       string
	Description string
}

func printUsage(version string, flags []CliFlag, availablePokemon []string) {
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

const POKEMON string = "pokemon"
const POKEMON_CLI_UPDATED string = "POKEMON_CLI_UPDATED"

// Injected at build time:
var Version string
var AvailablePokemon string
var UpdateUrl string

func main() {

	exe, err := os.Executable()

	if err != nil {
		panic(fmt.Sprintf("Error getting current executable dir: %v", err))
	}

	exe, err = filepath.Abs(exe)

	if err != nil {
		panic(fmt.Sprintf("Error getting current executable dir: %v", err))
	}

	exeDir := filepath.Dir(exe)

	isPosix := false
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "netbsd", "openbsd", "solaris":
		isPosix = true
	}

	if Version == "" {
		panic("Version must be specified in the build via `-ldflags \"-X 'main.Version=1.0.0'\"`")
	}

	if AvailablePokemon == "" {
		panic("At least one Pokemon must be specified in the build via `-ldflags \"-X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'\"`")
	}

	if UpdateUrl == "" {
		panic("Default UpdateUrl must be specified via in the build via `-ldflags \"-X 'main.UpdateUrl=https://localhost:8080/'\"`")
	}

	AvailablePokemon := strings.Split(AvailablePokemon, ",")

	for i := 0; i < len(AvailablePokemon); i += 1 {
		AvailablePokemon[i] = strings.ToLower(strings.TrimSpace(AvailablePokemon[i]))
	}

	helpFlag := CliFlag{
		Name:        "--help",
		Short:       "-h",
		Description: "Print this help message",
	}
	versionFlag := CliFlag{
		Name:        "--version",
		Short:       "-v",
		Description: "Print the version of this cli tool",
	}
	updateUrlFlag := CliFlag{
		Name:        "--update-url",
		Short:       "-u",
		Description: fmt.Sprintf("(optional) The url to obtain updates from (https is required). Defaults to %s", UpdateUrl),
	}
	
	var daemonIntervalSecs uint64 = 1
	daemonFlag := CliFlag{
		Name:        "--daemon",
		Short:       "-d",
		Description: fmt.Sprintf("(optional) Run this executable in daemon mode outputting a Pokemon greeting on an interval. Configure the interval in seconds by specifying an optional positive integer. Defaults to %d second if interval is unspecified", daemonIntervalSecs),
	}

	flags := []CliFlag{helpFlag, versionFlag, updateUrlFlag, daemonFlag}

	// TODO check for updates on startup.
	// TODO add flag to ignore updates on startup.

	var pokemon string
	args := os.Args

	daemonRun := false

	// Avoid using `flag` package here since we need to customize our arg parsing code.
	for i := 1; i < len(args); i += 1 {
		switch args[i] {
		case helpFlag.Name, helpFlag.Short:
			printUsage(Version, flags, AvailablePokemon)
			os.Exit(0)
			return
		case versionFlag.Name, versionFlag.Short:
			fmt.Fprintf(os.Stderr, "%s\n", Version)
			os.Exit(0)
			return
		case updateUrlFlag.Name, updateUrlFlag.Short:
			if i+1 < len(args) {
				i += 1
				UpdateUrl = args[i]
			} else {
				fmt.Fprint(os.Stderr, "No value provided for --update-url\n")
				printUsage(Version, flags, AvailablePokemon)
				os.Exit(128)
			}

			// Require https for user supplied URLs.
			// Allow non-https for the default URL for the sake of testing.
			if !strings.HasPrefix(UpdateUrl, "https:") {
				fmt.Fprintf(os.Stderr, "--update-url must use https: %s\n", UpdateUrl)
				printUsage(Version, flags, AvailablePokemon)
				os.Exit(128)
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
		default:
			if len(args[i]) == 0 || args[i][0] == '-' {
				fmt.Fprintf(os.Stderr, "Invalid flag: \"%s\"\n", args[i])
				printUsage(Version, flags, AvailablePokemon)
				os.Exit(128)
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

	type Versions struct {
		All []string `json:"versions"`
	}

	update := func() {

		fmt.Printf("Checking for updates...\n")
		resp, err := http.Get(fmt.Sprintf("%s/versions/%s", UpdateUrl, POKEMON))

		if err != nil {
			// TODO log more details or send info back to server
			fmt.Fprintf(os.Stderr, "Failed to check for updates: %v\n", err)

			return
		}

		versionsResp := resp
		defer versionsResp.Body.Close()

		var versions Versions

		if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil || len(versions.All) == 0 {
			// TODO log more details or send info back to server
			fmt.Fprintf(os.Stderr, "Failed to check for updates: %v\n", err)

			return
		}

		// TODO configure limits on versions to update.
		version := versions.All[len(versions.All)-1]

		if version == Version {
			return
		}

		updateFilePath := filepath.Join(exeDir, fmt.Sprintf("%s-%s", POKEMON, version))
		updateFile, err := os.Open(updateFilePath)

		if err != nil && errors.Is(err, os.ErrNotExist) {

			resp, err = http.Get(fmt.Sprintf("%s/downloads/%s?version=%s", UpdateUrl, POKEMON, version))

			if err != nil {
				// TODO log more details or send info back to server
				fmt.Fprintf(os.Stderr, "Failed to download update: %v\n", err)
				return
			}

			defer resp.Body.Close()

			// TODO possibly download to a temp file and atomic copy
			updateFile, err = os.Create(updateFilePath)

			if err != nil {
				// TODO log more details or send info back to server
				fmt.Fprintf(os.Stderr, "Failed to install update: %v\n", err)
				return
			}

			_, err = io.Copy(updateFile, resp.Body)

			if err != nil {
				updateFile.Close()

				// TODO log more details or send info back to server
				fmt.Fprintf(os.Stderr, "Failed to install update: %v\n", err)
				return
			}

			updateFile.Close()

		} else if err != nil {
			// TODO log more details or send info back to server
			fmt.Fprintf(os.Stderr, "Failed to open update file: %v\n", err)
			return
		} else {
			updateFile.Close()
		}

		if isPosix {
			info, err := os.Stat(updateFilePath)

			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to install update: %v\n", err)
				return
			}

			originalMode := info.Mode().Perm()

			// Add user execute permission to the original mode
			newMode := originalMode | 0o100

			// Apply the new permissions
			err = os.Chmod(updateFilePath, newMode)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to install update: %v\nManually mark the file as executable and restart the process to update:\nchmod +x \"%s\"\n", err, updateFilePath)
				return
			}

			err = os.Setenv(POKEMON_CLI_UPDATED, "TRUE")

			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to install update: %v\n", err)
				return
			}

			// TODO Exec doesn't exit, so I need to figure out what to do about defer calls
			err = syscall.Exec(updateFilePath, newArgs(os.Args, updateFilePath), os.Environ())

			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to start updated version: %v\n", err)
			} else {
				os.Exit(0)
			}
		}

		err = os.Setenv(POKEMON_CLI_UPDATED, "TRUE")

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to install update: %v\n", err)
			return
		}

		cmd := exec.Command(updateFilePath, newArgs(os.Args, updateFilePath)...)
		err = cmd.Run()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start updated version: %v\n", err)
			return
		} else {
			os.Exit(0)
		}
	}

	if os.Getenv(POKEMON_CLI_UPDATED) == "TRUE" {
		_ = os.Unsetenv(POKEMON_CLI_UPDATED)
		fmt.Printf("Successfully updated to %s\n", Version)
	} else {
		update()
	}

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

		// TODO periodically check for updates in a goroutine
	}
}
