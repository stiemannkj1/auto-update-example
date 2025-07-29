// Server which exposes CLI executable binaries for download. The server
// watches the filesystem to find new versions of the CLI tool.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/stiemannkj1/auto-update-example/common"
)

func printUsage(flags []common.CliFlag) {
	fmt.Fprintf(os.Stderr, "Usage: server\n\tStart a server that serves versions of the pokemon cli tool\n")

	for _, flag := range flags {
		fmt.Fprintf(os.Stderr, "%s, %s\n\t%s\n", flag.Name, flag.Short, flag.Description)
	}
}

// Server settings which can be configured via JSON file
type Settings struct {
	// The port for the server to listen on
	Port uint16
	// The directory containing the versions of executables available for download
	// .
	// ├── 1.0.0/
	// │     └── pokemon
	// │
	// ├── 2.0.0/
	// │     └──  pokemon
	// │
	// └── 3.0.0/
	//       └──  pokemon
	PokemonVersionDir string
	// The interval in seconds to wait before checking for new versions
	VersionCheckIntervalSecs uint64
	// The directory where logs files should be written. If empty, logs will be written to os.Stderr
	LogsDir string
	// The log level
	LogsLevel string
}

// Cache of version data to avoid unnecessary allocations and recalculations
// Use the Lock when reading and writing data otherwise access will not be
// thread-safe.
type VersionsCache struct {
	Versions           common.SemanticVersions
	Json               []byte
	VersionToSha512Map map[string]string
	Lock               sync.RWMutex
}

// Gets the Sha-512 hash for a particular version
func getSha512(versions *VersionsCache, version string) string {
	versions.Lock.RLock()
	defer versions.Lock.RUnlock()
	sha512, exists := versions.VersionToSha512Map[version]

	if exists {
		return sha512
	} else {
		return ""
	}
}

type VersionMessage struct {
	Msg     string `json:"message"`
	Version string `json:"version"`
}

// Reads a Json file into the provided value object. If the file is larger than
// maxSize, this method returns an error and the value struct is invalid
func readJsonFile[T any](filePath string, maxSize int64, value *T) error {

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	limitedReader := io.LimitReader(file, maxSize)

	decoder := json.NewDecoder(limitedReader)

	err = decoder.Decode(&value)
	if err != nil {
		return err
	}

	return nil
}

// Searches the filesystem for versions under settings.PokemonVersionsDir with
// the structure:
// .
// ├── 1.0.0/
// │     └── pokemon
// |
// ├── 2.0.0/
// │     └──  pokemon
// |
// └── 3.0.0/
//
//	└──  pokemon
//
// If the versions found are different than the previous version, this method
// updates the cache with the latest version information. Returns true if the
// cache was updated.
func updateVersions(logger *slog.Logger, settings *Settings, versions *VersionsCache) (updated bool, err error) {
	entries, err := os.ReadDir(settings.PokemonVersionDir)

	if err != nil {
		return false, err
	}

	// Find all versions and calculate all hashes prior to obtaining the locks
	// to minimize time spent holding the write lock.
	availableVersions := common.SemVers(make([]common.SemVer, 0, len(entries)))
	versionToSha512Map := make(map[string]string, len(entries))

	for _, entry := range entries {
		possibleVersion := entry.Name()

		version, err := common.ParseSemVer(possibleVersion)

		if err != nil {
			logger.Warn(fmt.Sprintf("Ignoring invalid version: %s", possibleVersion), "error", err)
			continue
		}

		path := filepath.Join(settings.PokemonVersionDir, entry.Name(), Pokemon)
		pokemonFile, err := os.Open(path)

		if err != nil && os.IsNotExist(err) {
			logger.Warn("Ignoring version with missing pokemon binary.", "file_name", path, "error", err)
			continue
		} else if err != nil {
			logger.Warn("Error reading pokemon binary.", "file_name", path, "error", err)
			continue
		}

		sha512, err := common.Sha512Hash(pokemonFile)
		pokemonFile.Close()

		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to obtain %s", common.Sha512Name), "file_name", path, "error", err)
			continue
		}

		versionToSha512Map[possibleVersion] = sha512

		availableVersions = append(availableVersions, version)
	}

	if maps.Equal(versionToSha512Map, versions.VersionToSha512Map) {
		return false, nil
	}

	sort.Sort(availableVersions)
	allVersions := common.SemanticVersions{
		All: availableVersions,
	}
	versionsJson, err := json.Marshal(&allVersions)

	if err != nil {
		logger.Warn(fmt.Sprintf("Unable to convert versions to JSON %s", allVersions), "error", err)
		return false, err
	}

	// Minimal write locking here to replace the old values.
	versions.Lock.Lock()
	defer versions.Lock.Unlock()

	versions.Versions = allVersions
	versions.VersionToSha512Map = versionToSha512Map
	versions.Json = versionsJson

	return true, nil
}

func logRequest(logger *slog.Logger, r *http.Request) {
	logger.Info("Request", "url", r.URL.String(), "method", r.Method, "ip address", r.RemoteAddr)
}

const Pokemon string = "pokemon"
const MB int64 = 1024 * 1024

func main() {

	// Define CLI args:
	helpFlag := common.CliFlag{
		Name:        "--help",
		Short:       "-h",
		Description: "Print this help message",
	}

	exampleSettings := Settings{
		Port:                     1234,
		PokemonVersionDir:        "/path/to/pokemon/versions/dir",
		VersionCheckIntervalSecs: 15,
		LogsDir:                  "/path/to/logs/dir",
		LogsLevel:                "WARN",
	}
	settingsJson, err := json.MarshalIndent(&exampleSettings, "\t", "\t")
	if err != nil {
		panic(fmt.Sprintf("Failed to build default %s JSON", reflect.TypeOf(exampleSettings).Name()))
	}

	settingsFlag := common.CliFlag{
		Name:        "--settings",
		Short:       "-s",
		Description: fmt.Sprintf("The JSON settings file for the server. You can configure the following settings in this file:\n\t%s", settingsJson),
	}

	flags := []common.CliFlag{helpFlag, settingsFlag}

	var settings Settings
	var settingsDir string

	args := os.Args

	// Parse CLI Args:
	for i := 1; i < len(args); i += 1 {
		switch args[i] {
		case helpFlag.Name, helpFlag.Short:
			printUsage(flags)
			return
		case settingsFlag.Name, settingsFlag.Short:
			if i+1 >= len(args) {
				break
			}

			i += 1
			settingsPath, err := filepath.Abs(args[i])

			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to find settings file \"%s\":\n%v\n\n", args[i], err)
				printUsage(flags)
				os.Exit(64)
			}

			err = readJsonFile(settingsPath, 1*MB, &settings)

			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to open settings file \"%s\":\n%v\n\n", settingsPath, err)
				printUsage(flags)
				os.Exit(64)
			}

			settingsDir = filepath.Dir(settingsPath)

			if settings.LogsDir != "" && !filepath.IsAbs(settings.LogsDir) {
				settings.LogsDir = filepath.Join(settingsDir, settings.LogsDir)
			}

			if !filepath.IsAbs(settings.PokemonVersionDir) {
				settings.PokemonVersionDir = filepath.Join(settingsDir, settings.PokemonVersionDir)
			}
		default:
			if len(args[i]) == 0 || args[i][0] == '-' {
				fmt.Fprintf(os.Stderr, "Invalid flag: \"%s\"\n\n", args[i])
				printUsage(flags)
				os.Exit(64)
			}
		}
	}

	if (Settings{}) == settings {
		fmt.Fprintf(os.Stderr, "No value provided for settings file\n\n")
		printUsage(flags)
		os.Exit(64)
	}

	// Initialize Logger.
	var logWriter io.Writer

	if settings.LogsDir == "" {
		logWriter = os.Stderr
	} else {

		err := os.MkdirAll(settings.LogsDir, 0b111111101)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log dirs \"%s\":\n%v\n\n", settings.LogsDir, err)
			printUsage(flags)
			os.Exit(1)
		}

		logFilePath := fmt.Sprintf("%s/pokemon-server.log", settings.LogsDir)
		logFile, err := os.Create(logFilePath)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open log file \"%s\" for writing:\n%v\n\n", logFilePath, err)
			printUsage(flags)
			os.Exit(1)
		}

		logWriter = bufio.NewWriter(logFile)
	}

	level, err := common.ToSlogLevel(settings.LogsLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: \"%s\". Defaulting to INFO.\n", settings.LogsLevel)
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(logWriter, &slog.HandlerOptions{
		Level: level,
	}))

	// Find CLI versions:
	versions := VersionsCache{}

	updated, err := updateVersions(logger, &settings, &versions)

	if !updated || err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find initial versions from pokemon version dir \"%s\":\n%v\n\n", settings.PokemonVersionDir, err)
		printUsage(flags)
		os.Exit(1)
	} else {
		logger.Info(fmt.Sprintf("Updated versions. Found: %s", versions.Versions))
	}

	// Initialize Endpoints:
	healthcheckHandler := func(w http.ResponseWriter, r *http.Request) {

		logRequest(logger, r)

		switch r.Method {
		case "GET", "HEAD":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}

	http.HandleFunc("/", healthcheckHandler)
	http.HandleFunc("/ping", healthcheckHandler)
	http.HandleFunc("/healthcheck", healthcheckHandler)

	// Versions enpoint that publishes the versions of the CLI tool which can
	// be downloaded:
	http.HandleFunc(fmt.Sprintf("/v1.0/versions/%s", Pokemon), func(w http.ResponseWriter, r *http.Request) {

		logRequest(logger, r)

		if r.Method != "GET" {
			w.WriteHeader(http.StatusForbidden)
		}

		w.Header().Add("Content-Type", "application/json")

		versions.Lock.RLock()
		defer versions.Lock.RUnlock()
		w.Write(versions.Json)
	})

	// Download endpoint which serves the CLI executable binary:
	http.HandleFunc(fmt.Sprintf("/v1.0/downloads/%s", Pokemon), func(w http.ResponseWriter, r *http.Request) {

		logRequest(logger, r)

		if r.Method != "GET" {
			w.WriteHeader(http.StatusForbidden)
		}

		version := r.URL.Query().Get("version")
		sha512 := getSha512(&versions, version)

		if sha512 == "" {
			w.WriteHeader(http.StatusNotFound)
			w.Header().Add("Content-Type", "application/json")
			json := json.NewEncoder(w)
			err := json.Encode(VersionMessage{
				Msg:     "The requested version does not exist.",
				Version: version,
			})

			if err != nil {
				logger.Warn("Error response failed for", "url", r.URL, "error", err)
			}

			return
		}

		w.Header().Add("Content-Type", "application/octet-stream")
		w.Header().Add("Content-Disposition", fmt.Sprintf("attachment; filename=pokemon-%s", version))
		w.Header().Add(common.Sha512Name, sha512)

		// TODO potentially cache the latest file in memory since it's the most
		// likely to be requested.
		http.ServeFile(w, r, fmt.Sprintf("%s/%s/pokemon", settings.PokemonVersionDir, version))
	})

	// Background thread to update versions. This thread may be killed at any
	// time, so don't expect any defer calls the complete. This should not be
	// used for writing external data to the filesystem.
	go func() {
		for {
			updated, err := updateVersions(logger, &settings, &versions)

			if err != nil {
				logger.Warn(fmt.Sprintf("Failed to update versions from %s", settings.PokemonVersionDir), "error", err)
			} else if updated {
				logger.Info(fmt.Sprintf("Updated versions. Found %s", versions.Versions))
			} else {
				logger.Info(fmt.Sprintf("No new versions found. Using existing versions: %s", versions.Versions))
			}

			time.Sleep(time.Duration(settings.VersionCheckIntervalSecs) * time.Second)
		}
	}()

	fmt.Printf("Listening on port: %d\n", settings.Port)
	err = http.ListenAndServe(fmt.Sprintf(":%d", settings.Port), nil)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server listening on %d:\n%v\n\n", settings.Port, err)
		os.Exit(1)
	}
}
