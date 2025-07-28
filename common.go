package common

import (
	"fmt"
	"log/slog"
	"strings"
)

type Versions struct {
	All []string `json:"versions"`
}

type CliFlag struct {
	Name        string
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
