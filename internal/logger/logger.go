package logger

import (
	"context"
	"sync"

	"emperror.dev/errors"
	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
)

const (
	KeyCmd        = "command"
	defaltAppName = "unknown"
)

var ErrInvalidConfig = errors.NewPlain("invalid config")

var loggers = sync.Map{} //nolint:gochecknoglobals // simple logging

func FromContext(ctx context.Context, keysAndValues ...interface{}) (context.Context, logr.Logger) {
	if ctx == nil {
		ctx = context.Background()
	}
	var log logr.Logger
	var err error
	var store bool
	if log, err = logr.FromContext(ctx); err != nil {
		log = GetLogger(defaltAppName)
		store = true
	}
	if len(keysAndValues) > 0 {
		log = log.WithValues(keysAndValues...)
		store = true
	}
	if store {
		ctx = logr.NewContext(ctx, log)
	}

	return ctx, log
}

func NewContext(ctx context.Context, log logr.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	return logr.NewContext(ctx, log)
}

func GetLogger(app string) logr.Logger {
	if logger, has := loggers.Load(app); has {
		return logger.(logr.Logger) //nolint:forcetypeassert // always logr.Logger
	}
	lr := logrus.New()
	lr.Level = logrus.TraceLevel
	logger := logrusr.New(lr).WithName(app)
	loggers.Store(app, logger)

	return logger
}
