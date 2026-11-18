package log

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
)

var (
	mu     sync.RWMutex
	logger *zap.Logger
)

func init() {
	_ = Init("info", "console")
	config.RegisterOnLoad(func(cfg *config.Config) error {
		return InitWithOptions(cfg.Log.Level, cfg.Log.Format, cfg.Log.ServiceName, "")
	})
}

// Init configures the global zap.Logger with level and format.
func Init(level, format string) error {
	return InitWithOptions(level, format, "", "")
}

// InitWithOptions configures the global zap.Logger to write structured logs to stderr.
func InitWithOptions(level, format, _, _ string) error {
	var zapLevel zapcore.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info", "":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		return fmt.Errorf("invalid log level %q", level)
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	var encoder zapcore.Encoder
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		encoder = zapcore.NewJSONEncoder(encCfg)
	case "console", "":
		encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	default:
		return fmt.Errorf("invalid log format %q", format)
	}

	stderrCore := zapcore.NewCore(encoder, zapcore.Lock(os.Stderr), zapLevel)
	z := zap.New(stderrCore)

	mu.Lock()
	logger = z
	mu.Unlock()

	zap.ReplaceGlobals(z)
	return nil
}

// L returns the global structured *zap.Logger.
func L() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

// S returns the global *zap.SugaredLogger.
func S() *zap.SugaredLogger {
	return L().Sugar()
}

// Info logs a message at InfoLevel with key-value pairs.
func Info(msg string, keysAndValues ...any) {
	S().Infow(msg, keysAndValues...)
}

// Warn logs a message at WarnLevel with key-value pairs.
func Warn(msg string, keysAndValues ...any) {
	S().Warnw(msg, keysAndValues...)
}

// Error logs a message at ErrorLevel with key-value pairs.
func Error(msg string, keysAndValues ...any) {
	S().Errorw(msg, keysAndValues...)
}

// Debug logs a message at DebugLevel with key-value pairs.
func Debug(msg string, keysAndValues ...any) {
	S().Debugw(msg, keysAndValues...)
}
