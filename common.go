// Common utilities shared between the Pokemon CLI and server
package common

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
)

const Sha512Name string = "Sha-512"

func IsPosix() bool {
	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "netbsd", "openbsd", "solaris":
		return true
	default:
		return false
	}
}

func Capitalize(s string) string {
	if s == "" {
		return s
	}

	return fmt.Sprintf("%s%s", strings.ToUpper(s[0:1]), s[1:])
}

// Version data used to communicate between client and server
type Versions struct {
	All []string `json:"versions"`
}

type CliFlag struct {
	// Long flag for the CLI arg such as "--switch"
	Name string
	// Short flag for the CLI arg such as "-s"
	Short       string
	Description string
}

func ToSlogLevel(levelStr string) (slog.Level, error) {
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

func ToHexHash(hasher *hash.Hash) string {
	hash := (*hasher).Sum(make([]byte, 0, 64))
	return hex.EncodeToString(hash)
}

// Obtain the Sha-512 hash of a file as a hexedecimal string
func Sha512Hash(file *os.File) (string, error) {

	hasher := sha512.New()

	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return ToHexHash(&hasher), nil
}

func NewSha512Error(path string, expectedSha512 string, sha512 string) error {
	return fmt.Errorf("expected file %s to have Sha-512 %s, but found %s", path, expectedSha512, sha512)
}
