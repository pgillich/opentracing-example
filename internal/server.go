package internal

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-logr/logr"
)

type ConfigSetter interface {
	SetListenAddr(string)
	SetInstance(string)
	SetCommand(string)
	SetJaegerURL(string)
	SetNatsURL(string)
	GetOptions() []string
}

var ErrInvalidServerRunner = errors.New("invalid server runner")

func RunServer(h http.Handler, shutdown <-chan struct{}, addr string, log logr.Logger) {
	server := &http.Server{ //nolint:gosec // not secure
		Handler: h,
		Addr:    addr,
	}

	go func() {
		<-shutdown
		if err := server.Shutdown(context.Background()); !errors.Is(err, http.ErrServerClosed) {
			log.Error(err, "Server shutdown error")
		}
	}()

	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Error(err, "Server exit error")
	} else {
		log.Info("Server exit")
	}
}

func (s *Frontend) writeErr(w http.ResponseWriter, statusCode int, err error) {
	w.WriteHeader(statusCode)
	if _, err := w.Write([]byte(err.Error())); err != nil { //nolint:govet // err shadow
		s.log.Error(err, "unable to write response")
	}
}
