package log

import (
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

func NewLogger(isDebug bool) *slog.Logger {
	if isDebug {
		handler := tint.NewHandler(os.Stdout, &tint.Options{
			Level: slog.LevelDebug,
		})
		return slog.New(handler)
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	})
	return slog.New(handler)
}
