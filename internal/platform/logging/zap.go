package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var global *zap.Logger

// Init initializes the global zap.Logger with the given level and format.
// Called once at application startup.
func Init(level, format string) error {
	var lvl zapcore.Level
	switch level {
	case "debug":
		lvl = zapcore.DebugLevel
	case "info":
		lvl = zapcore.InfoLevel
	case "warn":
		lvl = zapcore.WarnLevel
	case "error":
		lvl = zapcore.ErrorLevel
	default:
		lvl = zapcore.InfoLevel
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "ts"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	var encoder zapcore.Encoder
	if format == "console" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	var output zapcore.WriteSyncer
	// user could configure stderr, but default to stdout
	output = zapcore.AddSync(os.Stdout)

	core := zapcore.NewCore(encoder, output, zap.NewAtomicLevelAt(lvl))
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	global = logger
	return nil
}

// L returns the global zap.Logger. Must be called after Init.
func L() *zap.Logger {
	return global
}

// S returns a global SugaredLogger for Printf-style convenience. Must be called after Init.
func S() *zap.SugaredLogger {
	return global.Sugar()
}
