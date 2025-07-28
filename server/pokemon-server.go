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
)

type Versions struct {
	All []string `json:"versions"`
}

type CliFlag struct {
	Name        string
	Short       string
	Description string
}

func toSlogLevel(levelStr string) (slog.Level, error) {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown slog level: %s", levelStr)
	}
}

func printUsage(flags []CliFlag) {
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
	Versions Versions
	Json     []byte
	Set      map[string]bool
	Lock     sync.RWMutex
}

func versionExists(versions *VersionsCache, version string) bool {
	versions.Lock.RLock()
	defer versions.Lock.RUnlock()
	_, exists := versions.Set[version]
	return exists
}

type VersionMessage struct {
	Msg     string `json:"message"`
	Version string `json:"version"`
}

func toSet(strings []string) map[string]bool {
	stringSet := make(map[string]bool, len(strings))

	for _, s := range strings {
		stringSet[s] = true
	}

	return stringSet
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

	availableVersions := make([]string, 0, len(entries))

	for _, entry := range entries {
		possibleVersion := entry.Name()

		if !version.MatchString(possibleVersion) {
			logger.Warn(fmt.Sprintf("Ignoring invalid version: %s", possibleVersion))
			continue
		}

		path := filepath.Join(settings.PokemonVersionDir, entry.Name(), Pokemon)
		_, err = os.Open(path)

		if err != nil && os.IsNotExist(err) {
			logger.Warn(fmt.Sprintf("Ignoring version with missing pokemon binary: %s", path))
			continue
		} else if err != nil {
			logger.Warn(fmt.Sprintf("Error reading pokemon binary: %s", path))
			continue
		}

		availableVersions = append(availableVersions, possibleVersion)
	}

	if slices.Equal(availableVersions, versions.Versions.All) {
		return false, nil
	}

	allVersions := Versions{
		All: availableVersions,
	}
	versionsJson, err := json.Marshal(&allVersions)

	if err != nil {
		if logger.Enabled(context.Background(), slog.LevelWarn) {
			logger.Warn(fmt.Sprintf("Unable to convert versions to JSON: [%s]", strings.Join(availableVersions, ",")))
		}

		return false, err
	}

	versions.Lock.Lock()
	defer versions.Lock.Unlock()

	versions.Versions = allVersions
	versions.Set = toSet(availableVersions)
	versions.Json = versionsJson

	return true, nil
}

func logRequest(logger *slog.Logger, r *http.Request) {
	logger.Info("Request", "url", r.URL.String(), "method", r.Method, "ip address", r.RemoteAddr)
}

const Pokemon string = "pokemon"
const MB int64 = 1024 * 1024

var version *regexp.Regexp = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]$`)

func main() {

	helpFlag := CliFlag{
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

	settingsFlag := CliFlag{
		Name:        "--settings",
		Short:       "-s",
		Description: fmt.Sprintf("The JSON settings file for the server. You can configure the following settings in this file:\n\t%s", settingsJson),
	}

	flags := []CliFlag{helpFlag, settingsFlag}

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
				fmt.Fprintf(os.Stderr, "Unable to open settings file \"%s\":%v\n\n", args[i], err)
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
			fmt.Fprintf(os.Stderr, "Failed to create log dirs \"%s\": %v\n\n", settings.LogsDir, err)
			printUsage(flags)
			os.Exit(1)
		}

		logFilePath := fmt.Sprintf("%s/pokemon-server.log", settings.LogsDir)
		logFile, err := os.Create(logFilePath)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open log file \"%s\" for writing: %v\n\n", logFilePath, err)
			printUsage(flags)
			os.Exit(1)
		}

		logWriter = bufio.NewWriterSize(logFile, 512)
	}

	level, err := toSlogLevel(strings.ToUpper(settings.LogsLevel))
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
		fmt.Fprintf(os.Stderr, "Failed to find initial versions from pokemon version dir \"%s\": %v\n\n", settings.PokemonVersionDir, err)
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

		if !versionExists(&versions, version) {
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

		w.Header().Add("Content-Disposition", fmt.Sprintf("attachment; filename=pokemon-%s", version))

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
		fmt.Fprintf(os.Stderr, "Failed to start server listening on %d: %v\n\n", settings.Port, err)
		os.Exit(1)
	}
}
