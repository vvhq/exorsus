package logging

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"path"
	"path/filepath"
)

type FileHook struct {
	logger *logrus.Logger
	lumberLogger *lumberjack.Logger
	parentLogger *logrus.Logger
}

func (hook *FileHook) Rotate() {
	err := hook.lumberLogger.Rotate()
	if err != nil {
		hook.parentLogger.Error(err.Error())
	}
}

func (hook *FileHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (hook *FileHook) Fire(entry *logrus.Entry) error {
	switch entry.Level {
	case logrus.TraceLevel:
		hook.logger.WithFields(entry.Data).WithTime(entry.Time).Trace(entry.Message)
	case logrus.DebugLevel:
		hook.logger.WithFields(entry.Data).WithTime(entry.Time).Debug(entry.Message)
	case logrus.InfoLevel:
		hook.logger.WithFields(entry.Data).WithTime(entry.Time).Info(entry.Message)
	case logrus.WarnLevel:
		hook.logger.WithFields(entry.Data).WithTime(entry.Time).Warn(entry.Message)
	case logrus.ErrorLevel:
		hook.logger.WithFields(entry.Data).WithTime(entry.Time).Error(entry.Message)
	case logrus.FatalLevel:
		hook.logger.WithFields(entry.Data).WithTime(entry.Time).Fatal(entry.Message)
	case logrus.PanicLevel:
		hook.logger.WithFields(entry.Data).WithTime(entry.Time).Panic(entry.Message)
	default:
		hook.logger.WithFields(entry.Data).WithTime(entry.Time).Print(entry.Message)
	}
	return nil
}

func NewFileHook(parentLogger *logrus.Logger, logPath string, maxSize int, maxBackups int, maxAge int, localTime bool) (*FileHook, error) {
	fileLogger := logrus.New()
	fileLogger.SetLevel(parentLogger.Level)
	fileLogger.SetFormatter(&logrus.JSONFormatter{})
	fileLogger.SetNoLock()
	hostName, err := os.Hostname()
	if err == nil {
		logDirName, logFileName := filepath.Split(logPath)
		logPath = path.Join(logDirName, fmt.Sprintf("%s_%s", hostName, logFileName))
	} else {
		return nil, err
	}
	lumberLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		LocalTime:  localTime,
	}
	fileLogger.SetOutput(lumberLogger)
	return &FileHook{logger: fileLogger, lumberLogger: lumberLogger, parentLogger: parentLogger}, nil
}

func NewLogger(output io.Writer, level logrus.Level) *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(level)
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true, DisableLevelTruncation: true})
	logger.SetOutput(output)
	logger.SetNoLock()
	if logger.Level == logrus.TraceLevel {
		logger.SetReportCaller(true)
	}
	return logger
}
