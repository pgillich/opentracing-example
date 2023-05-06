package logger

import (
	"errors"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
)

const (
	KeyCmd = "command"
)

var ErrInvalidConfig = errors.New("invalid config")

var loggers = map[string]logr.Logger{} // nolint:gochecknoglobals // simple logging

func GetLogger(app string) logr.Logger {
	if logger, has := loggers[app]; has {
		return logger
	}
	lr := logrus.New()
	lr.Level = logrus.TraceLevel
	loggers[app] = logrusr.New(lr).WithName(app)

	return loggers[app]
}
