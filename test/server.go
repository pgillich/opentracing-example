package test

import (
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	"github.com/pgillich/opentracing-example/internal/model"
)

func TestServerRunner(server *httptest.Server, started chan struct{}) model.ServerRunner {
	return func(h http.Handler, shutdown <-chan struct{}, addr string, log logr.Logger) {
		server.Config.Handler = h
		log.Info("TestServer start")
		server.Start()
		close(started)
		log.Info("TestServer started")
		<-shutdown
		log.Info("TestServer shutdown")
		server.Close()
	}
}
