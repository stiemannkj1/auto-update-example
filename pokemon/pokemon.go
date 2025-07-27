package main

import (
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strconv"
	"strings"
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

// Injected at build time:
var Version string
var AvailablePokemon string

// End

func main() {

	if Version == "" {
		panic("Version must be specified in the build via `-ldflags \"-X 'main.Version=1.0.0'\"`")
	}

	if AvailablePokemon == "" {
		panic("At least one Pokemon must be specified in the build via `-ldflags \"-X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'\"`")
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
	daemonFlag := CliFlag{
		Name:        "--daemon",
		Short:       "-d",
		Description: "(optional) Run this executable in daemon mode outputting a Pokemon greeting on an interval. Configure the interval in seconds by specifying an optional positive integer",
	}

	flags := []CliFlag{helpFlag, versionFlag, daemonFlag}

	// TODO check for updates on startup.
	// TODO add flag to ignore updates on startup.

	var pokemon string
	args := os.Args
	daemonRun := false
	var daemonIntervalSecs uint64 = 1

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
