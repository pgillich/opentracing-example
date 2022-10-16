package internal

import (
	"context"
	"net/http"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
)

var ErrInvalidServerRunner = errors.NewPlain("invalid server runner")

func RunServer(h http.Handler, shutdown <-chan struct{}, addr string, log logr.Logger) {
	server := &http.Server{ // nolint:gosec // not secure
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
