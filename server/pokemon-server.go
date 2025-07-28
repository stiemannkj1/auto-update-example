package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
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

type Settings struct {
	Port                     uint16
	PokemonVersionDir        string
	VersionCheckIntervalSecs uint64
	LogsDir                  string
	LogsLevel                string
}

type VersionsCache struct {
	Versions           common.Versions
	Json               []byte
	VersionToSha512Map map[string]string
	Lock               sync.RWMutex
}

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

func updateVersions(logger *slog.Logger, settings *Settings, versions *VersionsCache) (updated bool, err error) {
	entries, err := os.ReadDir(settings.PokemonVersionDir)

	if err != nil {
		return false, err
	}

	// Find all versions and calculate all hashes prior to obtaining the locks
	// to minimize time spent holding the write lock.
	availableVersions := make([]string, 0, len(entries))
	versionToSha512Map := make(map[string]string, len(entries))

	for _, entry := range entries {
		possibleVersion := entry.Name()

		if !VersionRegex.MatchString(possibleVersion) {
			logger.Warn(fmt.Sprintf("Ignoring invalid version: %s", possibleVersion))
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

		availableVersions = append(availableVersions, possibleVersion)
	}

	if slices.Equal(availableVersions, versions.Versions.All) {
		return false, nil
	}

	allVersions := common.Versions{
		All: availableVersions,
	}
	versionsJson, err := json.Marshal(&allVersions)

	if err != nil {
		if logger.Enabled(context.Background(), slog.LevelWarn) {
			logger.Warn(fmt.Sprintf("Unable to convert versions to JSON [%s]", strings.Join(availableVersions, ",")), "error", err)
		}

		return false, err
	}

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

var VersionRegex *regexp.Regexp = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]$`)

func main() {

	helpFlag := common.CliFlag{
		Name:        "--help",
		Short:       "-h",
		Description: "Print this help message",
	}

	emptySettings := Settings{
		Port:                     1234,
		PokemonVersionDir:        "/path/to/pokemon/versions/dir",
		VersionCheckIntervalSecs: 15,
		LogsDir:                  "/path/to/logs/dir",
		LogsLevel:                "WARN",
	}
	settingsJson, err := json.MarshalIndent(&emptySettings, "\t", "\t")
	if err != nil {
		panic(fmt.Sprintf("Failed to build default %s JSON", reflect.TypeOf(emptySettings).Name()))
	}

	settingsFlag := common.CliFlag{
		Name:        "--settings",
		Short:       "-s",
		Description: fmt.Sprintf("The JSON settings file for the server. You can configure the following settings in this file:\n\t%s", settingsJson),
	}

	flags := []common.CliFlag{helpFlag, settingsFlag}

	var settings Settings

	args := os.Args

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
			err := readJsonFile(args[i], 1*MB, &settings)

			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to open settings file \"%s\":\n%v\n\n", args[i], err)
				printUsage(flags)
				os.Exit(64)
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

		logWriter = bufio.NewWriterSize(logFile, 512)
	}

	level, err := common.ToSlogLevel(settings.LogsLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: \"%s\". Defaulting to INFO.\n", settings.LogsLevel)
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(logWriter, &slog.HandlerOptions{
		Level: level,
	}))
	versions := VersionsCache{}

	updated, err := updateVersions(logger, &settings, &versions)

	if !updated || err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find initial versions from pokemon version dir \"%s\":\n%v\n\n", settings.PokemonVersionDir, err)
		printUsage(flags)
		os.Exit(1)
	} else if logger.Enabled(context.Background(), slog.LevelInfo) {
		logger.Info(fmt.Sprintf("Updated versions. Found [%s]", strings.Join(versions.Versions.All, ",")))
	}

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

	http.HandleFunc(fmt.Sprintf("/versions/%s", Pokemon), func(w http.ResponseWriter, r *http.Request) {

		logRequest(logger, r)

		if r.Method != "GET" {
			w.WriteHeader(http.StatusForbidden)
		}

		w.Header().Add("Content-Type", "application/json")

		versions.Lock.RLock()
		defer versions.Lock.RUnlock()
		w.Write(versions.Json)
	})

	http.HandleFunc(fmt.Sprintf("/downloads/%s", Pokemon), func(w http.ResponseWriter, r *http.Request) {

		logRequest(logger, r)

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
			} else if logger.Enabled(context.Background(), slog.LevelInfo) {

				if updated {
					logger.Info(fmt.Sprintf("Updated versions. Found [%s]", strings.Join(versions.Versions.All, ",")))
				} else {
					logger.Info(fmt.Sprintf("No new versions found. Using existing versions: [%s]", strings.Join(versions.Versions.All, ",")))
				}
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
