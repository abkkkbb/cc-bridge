package logger

import (
	"log"
	"strings"
)

// Level represents log severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var currentLevel = LevelInfo

// SetLevel sets the global log level from a string ("debug", "info", "warn", "error").
func SetLevel(s string) {
	switch strings.ToLower(s) {
	case "debug":
		currentLevel = LevelDebug
	case "warn", "warning":
		currentLevel = LevelWarn
	case "error":
		currentLevel = LevelError
	default:
		currentLevel = LevelInfo
	}
}

func Debug(format string, args ...any) {
	if currentLevel <= LevelDebug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func Info(format string, args ...any) {
	if currentLevel <= LevelInfo {
		log.Printf("[INFO] "+format, args...)
	}
}

func Warn(format string, args ...any) {
	if currentLevel <= LevelWarn {
		log.Printf("[WARN] "+format, args...)
	}
}

func Error(format string, args ...any) {
	if currentLevel <= LevelError {
		log.Printf("[ERROR] "+format, args...)
	}
}
