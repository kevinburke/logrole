package test

import "log/slog"

var NullLogger = slog.New(slog.DiscardHandler)
