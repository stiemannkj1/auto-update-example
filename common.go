// Common utilities shared between the Pokemon CLI and server
package common

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"math"
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

type SemVer struct {
	Major  uint64
	Minor  uint64
	Patch  uint64
	String string
}

type Ebyte byte

func e(e byte) uint64 {

	var value uint64
	value = 1

	if e == 0 {
		return value
	}

	for i := 0; i < int(e); i++ {
		value *= 10
	}

	return value
}

func ParseSemVer(version string) (SemVer, error) {

	var semVer SemVer
	semVer.String = version
	var subVersion uint64
	var i byte

	size := len(version)

	if size > math.MaxUint8 {
		return SemVer{}, fmt.Errorf("version %s too large", version)
	} else if size < len("0.0.0") {
		return SemVer{}, fmt.Errorf("version %s too small", version)
	}

	var lastDotIndex byte
	lastDotIndex = byte(size - 1)
	subVersionIndex := 0
	requireDigit := true

	for i = lastDotIndex; ; i -= 1 {
		if !requireDigit && version[i] == '.' && i > 0 {
			switch subVersionIndex {
			case 0:
				semVer.Patch = subVersion
			case 1:
				semVer.Minor = subVersion
			default:
				return SemVer{}, fmt.Errorf("too many version sections in %s; extra section starts at %d", version, i)
			}
			lastDotIndex = i - 1
			subVersion = 0
			subVersionIndex += 1
			requireDigit = true
		} else if '0' <= version[i] && version[i] <= '9' {
			subVersion += uint64(version[i]-byte('0')) * e(lastDotIndex-i)
			requireDigit = false
		} else {
			return SemVer{}, fmt.Errorf("%s was not a semantic version; invalid character %c at %d", version, version[i], i)
		}

		if i == 0 {
			break
		}
	}

	semVer.Major = subVersion

	const MAX_SUBVERSIONS = 3

	if subVersionIndex == (MAX_SUBVERSIONS - 1) {
		return semVer, nil
	}

	return SemVer{}, fmt.Errorf("%s was truncated; expected %d version sections", version, MAX_SUBVERSIONS)
}

func (v SemVer) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String)
}

type SemVers []SemVer

func (a SemVers) Len() int {
	return len(a)
}

func (a SemVers) Less(i, j int) bool {

	if a[i].Major < a[j].Major {
		return true
	}

	if a[i].Major == a[j].Minor &&
		a[i].Minor < a[j].Minor {
		return true
	}

	if a[i].Major == a[j].Minor &&
		a[i].Minor == a[j].Minor &&
		a[i].Patch < a[j].Patch {
		return true
	}

	return false
}

func (a SemVers) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// Version data used to communicate between client and server
type Versions struct {
	// Versions in ascending order
	All []string `json:"versions"`
}

// Version data used to communicate between client and server
type SemanticVersions struct {
	// Versions in ascending order
	All []SemVer `json:"versions"`
}

func (versions SemanticVersions) String() string {

	var builder strings.Builder
	builder.WriteRune('[')

	for i, version := range versions.All {

		if i > 0 {
			builder.WriteRune(',')
		}

		builder.WriteString(version.String)
	}

	builder.WriteRune(']')
	return builder.String()
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
