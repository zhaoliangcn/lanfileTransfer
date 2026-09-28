package utils

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var Log *zap.Logger
var SugaredLog *zap.SugaredLogger

// Defaults to a no-op logger so logging is always safe to call. Error paths in
// the transfer manager run before (or without) InitLogger -- in tests, and during
// early startup -- and a nil SugaredLog there would turn a handled error into a
// nil-pointer panic.
func init() {
	Log = zap.NewNop()
	SugaredLog = Log.Sugar()
}

func InitLogger(level string) {
	var logLevel zapcore.Level
	switch level {
	case "debug":
		logLevel = zapcore.DebugLevel
	case "info":
		logLevel = zapcore.InfoLevel
	case "warn":
		logLevel = zapcore.WarnLevel
	case "error":
		logLevel = zapcore.ErrorLevel
	default:
		logLevel = zapcore.InfoLevel
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		logLevel,
	)

	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	SugaredLog = Log.Sugar()
}

func SyncLogger() {
	if Log != nil {
		_ = Log.Sync()
	}
}
