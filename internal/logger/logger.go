package logger

import (
	"emperror.dev/errors"
	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
)

const (
	KeyCmd = "command"
)

var ErrInvalidConfig = errors.NewPlain("invalid config")

var loggers = map[string]logr.Logger{} // nolint:gochecknoglobals // simple logging

func GetLogger(app string) logr.Logger {
	if logger, has := loggers[app]; has {
		return logger
	}
	loggers[app] = logrusr.New(logrus.New()).WithName(app)

	return loggers[app]
}
