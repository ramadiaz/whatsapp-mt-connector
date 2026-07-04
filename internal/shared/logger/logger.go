package logger

import (
	"os"

	"github.com/rs/zerolog"
)

var Log zerolog.Logger

func Init(env string) {
	Log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "2006-01-02 15:04:05"}).With().Timestamp().Caller().Logger()
}

func WithCorrelationID(correlationID string) zerolog.Logger {
	return Log.With().Str("correlation_id", correlationID).Logger()
}
