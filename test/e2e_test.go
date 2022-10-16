package test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/pgillich/opentracing-example/cmd"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/stretchr/testify/suite"
)

type E2ETestSuite struct {
	suite.Suite
}

func TestExampleTestSuite(t *testing.T) {
	suite.Run(t, new(E2ETestSuite))
}

type TestServer struct {
	testServer *httptest.Server
	addr       string
	ctx        context.Context //nolint:containedctx // test
	cancel     context.CancelFunc
}

func (s *E2ETestSuite) TestMoreBackend() {
	log := logger.GetLogger(s.T().Name())

	beServer1 := runTestServer("backend", "--response", "PONG_1")
	defer beServer1.cancel()
	beServer2 := runTestServer("backend", "--response", "PONG_2")
	defer beServer2.cancel()
	feServer1 := runTestServer("frontend")
	defer feServer1.cancel()

	s.sendPing(feServer1, beServer1.addr, log)
	s.sendPing(feServer1, beServer2.addr, log)

	time.Sleep(1 * time.Second)
}

func (s *E2ETestSuite) sendPing(feServer *TestServer, beServerAddr string, log logr.Logger) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+feServer.addr+"/proxy", strings.NewReader("http://"+beServerAddr+"/ping"))
	s.NoError(err, "ping req")
	resp, err := feServer.testServer.Client().Do(req)
	s.NoError(err, "ping do")
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	s.NoError(err, "ping body")
	log.Info("Client resp", "body", string(body))
}

func runTestServer(typeName string, args ...string) *TestServer {
	server := &TestServer{
		testServer: httptest.NewUnstartedServer(nil),
	}
	server.addr = server.testServer.Listener.Addr().String()
	started := make(chan struct{})
	runner := TestServerRunner(server.testServer, started)
	server.ctx, server.cancel = context.WithCancel(context.Background())
	go cmd.Execute(server.ctx, append([]string{typeName, "--listenaddr", server.addr}, args...), runner)
	<-started
	time.Sleep(1 * time.Second)

	return server
}
