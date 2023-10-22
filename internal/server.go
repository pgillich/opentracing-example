package internal

import (
	"context"
	"log/slog"
	"net/http"

	"emperror.dev/errors"
	"github.com/pgillich/opentracing-example/internal/logger"
)

type ConfigSetter interface {
	SetListenAddr(string)
	SetInstance(string)
	SetCommand(string)
	SetJaegerURL(string)
	GetOptions() []string
}

var ErrInvalidServerRunner = errors.NewPlain("invalid server runner")

func RunServer(h http.Handler, shutdown <-chan struct{}, addr string, log *slog.Logger) {
	server := &http.Server{ // nolint:gosec // not secure
		Handler: h,
		Addr:    addr,
	}

	go func() {
		<-shutdown
		if err := server.Shutdown(context.Background()); !errors.Is(err, http.ErrServerClosed) {
			log.Error("Server shutdown error", logger.KeyError, err)
		}
	}()

	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Error("Server exit error", logger.KeyError, err)
	} else {
		log.Info("Server exit")
	}
}

func (s *Frontend) writeErr(w http.ResponseWriter, statusCode int, err error) {
	w.WriteHeader(statusCode)
	if _, err := w.Write([]byte(err.Error())); err != nil { //nolint:govet // err shadow
		s.log.Error("unable to write response", logger.KeyError, err)
	}
}
