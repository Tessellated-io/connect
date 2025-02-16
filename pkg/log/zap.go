package log

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2" // Include this for lumberjack
)

type LogOutputFormat int

const (
	LogOutputFormatJson LogOutputFormat = iota
	LogOutputFormatPlain
)

// DefaultOutputFormat is the default format that will be used for logging output.
const DefaultOutputFormat = LogOutputFormatJson

// logFormatEncodingMapping is a llist of stringified format values, indexable by a LogOutputFormat
var logFormatEncodingMapping = []string{"json", "plain"}

// ValidLogFormatOptions returns a stringified list of valid options, suitable for use in documentation
func ValidLogFormatOptions() string {
	output := ""
	for idx, logFormat := range logFormatEncodingMapping {
		output += logFormat
		if idx != len(logFormatEncodingMapping)-1 {
			output += ", "
		}
	}

	return output
}

func (l LogOutputFormat) String() string {
	return logFormatEncodingMapping[l]
}

func (l LogOutputFormat) Encoder(encoderCfg zapcore.EncoderConfig) zapcore.Encoder {
	var encoder zapcore.Encoder
	switch l {
	case LogOutputFormatJson:
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	case LogOutputFormatPlain:
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	}

	return encoder
}

// FromStringOrDefault attempts to parse a user input into a LogOutputFormat.
// If the given input is unparseable or invalid, this file prints an error to stdout and returns DefaultOutputFormat.
func FromStringOrDefault(input string) LogOutputFormat {
	// Strip whitespace and convert to lowercase
	normalized := strings.ToLower(strings.TrimSpace(input))

	// Attempt to match against a known format
	for outputFormatIdx, outputFormat := range logFormatEncodingMapping {
		if strings.EqualFold(outputFormat, normalized) {
			return LogOutputFormat(outputFormatIdx)
		}
	}

	// Unable to match, print error and use default
	// Cannot print using a logger because logging is not yet set up.
	fmt.Printf("unable to parse logging format \"%s\", defaulting to \"%s\"\n", input, DefaultOutputFormat.String())

	return DefaultOutputFormat
}

// Config is the configuration for the logger.
type Config struct {
	// StdOutLogLevel is the log level for the standard out logger.
	StdOutLogLevel string
	// FileOutLogLevel is the log level for the file logger.
	FileOutLogLevel string
	// DisableRotating disables log rotation.
	DisableRotating bool
	// WriteTo is the output file for the logger. If empty, logs will be written to stderr.
	WriteTo string
	// MaxSize is the maximum size in megabytes before log is rotated.
	MaxSize int
	// MaxBackups is the maximum number of old log files to retain.
	MaxBackups int
	// MaxAge is the maximum number of days to retain an old log file.
	MaxAge int
	// Compress determines if the rotated log files should be compressed.
	Compress bool
	// StdOutOutputFormat is the output format for a the file logger
	StdOutOutputFormat LogOutputFormat
	// FileOutputFormat is the output format for a the file logger
	FileOutputFormat LogOutputFormat
}

// NewDefaultConfig creates a default configuration for the logger.
func NewDefaultConfig() Config {
	return Config{
		StdOutLogLevel:     "info",
		FileOutLogLevel:    "info",
		DisableRotating:    false,
		WriteTo:            "sidecar.log",
		MaxSize:            1, // 100MB
		MaxBackups:         1,
		MaxAge:             3, // 3 days
		Compress:           false,
		StdOutOutputFormat: DefaultOutputFormat,
		FileOutputFormat:   DefaultOutputFormat,
	}
}

func NewLogger(config Config) *zap.Logger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	var fileCore zapcore.Core
	if config.WriteTo != "" && !config.DisableRotating {
		// Configure lumberjack for logging to a file
		lumberjackLogger := &lumberjack.Logger{
			Filename:   config.WriteTo,
			MaxSize:    config.MaxSize,
			MaxBackups: config.MaxBackups,
			MaxAge:     config.MaxAge,
			Compress:   config.Compress,
		}
		fileSyncer := zapcore.AddSync(lumberjackLogger)

		logLevel := zapcore.InfoLevel
		if err := logLevel.Set(config.FileOutLogLevel); err != nil {
			fmt.Fprintf(os.Stderr, "failed to set log level on file logging: %v\nfalling back to info", err)
			logLevel = zapcore.InfoLevel // Fallback to info if setting fails
		}

		fileCore = zapcore.NewCore(
			config.FileOutputFormat.Encoder(encoderCfg),
			fileSyncer,
			logLevel,
		)
	}

	// Setup the primary output to always include os.Stderr.
	logLevel := zapcore.InfoLevel
	if err := logLevel.Set(config.StdOutLogLevel); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set log level on std out: %v\nfalling back to info", err)
		logLevel = zapcore.InfoLevel // Fallback to info if setting fails
	}

	// Setup the primary output to always include os.Stderr
	stdCore := zapcore.NewCore(
		config.StdOutOutputFormat.Encoder(encoderCfg),
		zapcore.Lock(os.Stderr),
		logLevel,
	)

	// Use zapcore.NewTee to write to both stderr and the file (if configured)
	var core zapcore.Core
	if fileCore != nil {
		core = zapcore.NewTee(stdCore, fileCore)
	} else {
		core = stdCore
	}

	return zap.New(
		core,
		zap.AddCaller(),
		zap.Fields(zapcore.Field{Key: "pid", Type: zapcore.Int64Type, Integer: int64(os.Getpid())}),
	)
}
