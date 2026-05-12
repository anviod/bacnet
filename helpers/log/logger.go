package log

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the global zap logger instance used throughout the project.
var Logger *zap.Logger

func init() {
	initLogger()
}

func initLogger() {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		zapcore.DebugLevel,
	)
	Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0))
}

// NewNop returns a no-op logger, useful for tests.
func NewNop() *zap.Logger {
	return zap.NewNop()
}

// Sync flushes any buffered log entries. Should be called before exit.
func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}
