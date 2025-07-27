package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
)

type CliFlag struct {
	Name        string
	Short       string
	Description string
}

func printUsage(flags []CliFlag) {
	fmt.Fprintf(os.Stderr, "Usage: server\n")

	for _, flag := range flags {
		fmt.Fprintf(os.Stderr, "%s, %s\n\t%s\n", flag.Name, flag.Short, flag.Description)
	}
}

func healthcheckHandler(w http.ResponseWriter, r *http.Request) {

	switch r.Method {
	case "GET", "HEAD":
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusForbidden)
	}
}

type Versions struct {
	All  []string            `json:"versions"`
	Json []byte              `json:"-"`
	Set  map[string]struct{} `json:"-"`
	Lock sync.RWMutex        `json:"-"`
}

type VersionMessage struct {
	Msg     string `json:"message"`
	Version string `json:"version"`
}

func toSet(strings []string) map[string]struct{} {
	stringSet := make(map[string]struct{})

	for _, s := range strings {
		stringSet[s] = struct{}{}
	}

	return stringSet
}

func main() {

	helpFlag := CliFlag{
		Name:        "--help",
		Short:       "-h",
		Description: "Print this help message",
	}
	portFlag := CliFlag{
		Name:        "--port",
		Short:       "-p",
		Description: "The port for the server to listen on",
	}

	flags := []CliFlag{helpFlag, portFlag}

	var port uint64 = 8080

	args := os.Args

	for i := 1; i < len(args); i += 1 {
		switch args[i] {
		case helpFlag.Name, helpFlag.Short:
			printUsage(flags)
			os.Exit(1)
			return
		case portFlag.Name, portFlag.Short:
			var err error

			if i+1 < len(args) {
				i += 1
				port, err = strconv.ParseUint(args[i], 10, 16)
			} else {
				fmt.Fprint(os.Stderr, "No value provided for port\n")
				printUsage(flags)
				os.Exit(128)
			}

			if err != nil {
				fmt.Fprintf(os.Stderr, "Invalid value for port \"%s\"\n", args[i+1])
				printUsage(flags)
				os.Exit(128)
			}
		default:
			if len(args[i]) == 0 || args[i][0] == '-' {
				fmt.Fprintf(os.Stderr, "Invalid flag: \"%s\"\n", args[i])
				printUsage(flags)
				os.Exit(128)
			}
		}
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	versions := Versions{
		All: []string{
			"1.0.0",
			"2.0.0",
			"3.0.0",
		},
	}

	versions.Set = toSet(versions.All)

	var err error
	versions.Json, err = json.Marshal(&versions)

	ctx := context.Background()

	if err != nil && logger.Enabled(ctx, slog.LevelWarn) {
		logger.Warn("Unable to convert initial versions to JSON:", "versions", versions.All)
	}

	// TODO log all request info if debug is enabled.

	http.HandleFunc("/ping", healthcheckHandler)
	http.HandleFunc("/healthcheck", healthcheckHandler)

	versionsHandler := func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(os.Stderr, r.URL.Path)
		if r.Method != "GET" {
			w.WriteHeader(http.StatusForbidden)
		}

		w.Header().Add("Content-Type", "application/json")

		versions.Lock.RLock()
		defer versions.Lock.RUnlock()
		w.Write(versions.Json)
	}

	http.HandleFunc("/versions", versionsHandler)
	http.HandleFunc("/versions/", versionsHandler)

	downloadHandler := func(w http.ResponseWriter, r *http.Request) {
		version := r.URL.Query().Get("version")

		versions.Lock.RLock()
		defer versions.Lock.RUnlock()

		if _, ok := versions.Set[version]; !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Header().Add("Content-Type", "application/json")
			json := json.NewEncoder(w)
			err := json.Encode(VersionMessage{
				Msg:     "The requested version does not exist.",
				Version: version,
			})

			if err != nil {
				logger.Warn("Error response failed for", "url", r.URL)
			}

			return
		}

		w.Header().Add("Content-Disposition", "attachment; filename=pokemon")
		// TODO potentially cache the latest file in memory since it's the most likely to be requested.
		// TODO read versions from config file or DB
		http.ServeFile(w, r, fmt.Sprintf("/Volumes/Projects/stiemannkj1/auto-update-example/pokemon/version/%s/pokemon", version))
	}

	http.HandleFunc("/download/pokemon", downloadHandler)

	// TODO find new versions on the file system. Read versions dir from config file.
	// TODO auth for new versions?

	fmt.Printf("Listening on port: %d\n", port)
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
