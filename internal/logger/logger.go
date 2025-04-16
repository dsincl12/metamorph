package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Initialize sets up the global logger with the specified settings
func Initialize(debug bool) {
	// Pretty print logs in development
	if debug {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	}

	// Set global log level
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// Add caller info to log
	log.Logger = log.With().Caller().Logger()
}

// Get returns the global logger instance
func Get() *zerolog.Logger {
	return &log.Logger
}
