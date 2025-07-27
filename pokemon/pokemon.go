package main

import (
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strings"
	"time"
)

type CliFlag struct {
	Name        string
	Short       string
	Description string
}

func printUsage(flags []CliFlag, availablePokemon []string) {
	fmt.Fprintf(os.Stderr, "Usage: pokemon [(optional) pokemon name]\n\n")

	for _, flag := range flags {
		fmt.Fprintf(os.Stderr, "%s, %s\n\t%s\n", flag.Name, flag.Short, flag.Description)
	}

	fmt.Fprintf(os.Stderr, "\nSupported pokemon:\n\n")

	for _, pokemon := range availablePokemon {
		fmt.Fprintf(os.Stderr, "\t- %s%s\n", strings.ToUpper(pokemon[0:1]), pokemon[1:])
	}

	fmt.Fprintln(os.Stderr)
}

var AvailablePokemon string // Injected at build time.

func main() {

	if AvailablePokemon == "" {
		panic("At least one pokemon must be specified in the build via `-ldflags \"-X 'main.AvailablePokemon=pikachu,charmander,squirtle,bulbasaur'\"`")
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
	daemonFlag := CliFlag{
		Name:        "--daemon",
		Short:       "-d",
		Description: "(optional) Run this executable in daemon mode",
	}

	flags := []CliFlag{helpFlag, daemonFlag}

	// TODO check for updates on startup.
	// TODO add flag to ignore updates on startup.

	var pokemon string
	args := os.Args
	runDaemon := false

	for i := 1; i < len(args); i += 1 {
		switch args[i] {
		case helpFlag.Name, helpFlag.Short:
			printUsage(flags, AvailablePokemon)
			os.Exit(1)
			return
		case daemonFlag.Name, daemonFlag.Short:
			runDaemon = true
		default:
			if len(args[i]) == 0 || args[i][0] == '-' {
				fmt.Fprintf(os.Stderr, "Invalid flag: \"%s\"\n", args[i])
				printUsage(flags, AvailablePokemon)
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
		fmt.Fprintf(os.Stderr, "%s is not a supported pokemon.\n", pokemon)
		return
	}

	for {

		if randomPokemon {
			pokemon = AvailablePokemon[rand.Intn(len(AvailablePokemon))]
		}

		fmt.Printf("%s%s says, \"Hi!\".\n", strings.ToUpper(pokemon[0:1]), pokemon[1:])

		if !runDaemon {
			return
		} else {
			time.Sleep(1 * time.Second)
		}

		// TODO periodically check for updates in a goroutine
	}
}
