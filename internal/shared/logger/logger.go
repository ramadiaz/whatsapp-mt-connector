package logger

import (
	"os"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

func Init(env string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if env != "production" {
		Log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Caller().Logger()
		return
	}
	Log = zerolog.New(os.Stdout).With().Timestamp().Logger()
}

func WithCorrelationID(correlationID string) zerolog.Logger {
	return Log.With().Str("correlation_id", correlationID).Logger()
}
