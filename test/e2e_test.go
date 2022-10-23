package test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
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

type TestClient struct {
	addr   string
	ctx    context.Context //nolint:containedctx // test
	cancel context.CancelFunc
}

func (s *E2ETestSuite) TestMoreBackendFromFrontend() {
	log := logger.GetLogger(s.T().Name())

	beServer1 := runTestServer("backend", s.T().Name()+"#backend:1", "--response", "PONG_1")
	defer beServer1.cancel()
	beServer2 := runTestServer("backend", s.T().Name()+"#backend:2", "--response", "PONG_2")
	defer beServer2.cancel()
	feServer1 := runTestServer("frontend", s.T().Name()+"#backend-1")
	defer feServer1.cancel()

	s.sendPingFrontend(feServer1, []string{beServer1.addr}, log)
	s.sendPingFrontend(feServer1, []string{beServer1.addr, beServer2.addr, beServer2.addr}, log)

	time.Sleep(1 * time.Second)
}

func (s *E2ETestSuite) sendPingFrontend(feServer *TestServer, beServerAddrs []string, log logr.Logger) {
	for a := range beServerAddrs {
		beServerAddrs[a] = "http://" + beServerAddrs[a] + "/ping"
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+feServer.addr+"/proxy", strings.NewReader(strings.Join(beServerAddrs, " ")))
	s.NoError(err, "ping req")
	resp, err := feServer.testServer.Client().Do(req)
	s.NoError(err, "ping do")
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	s.NoError(err, "ping body")
	log.Info("Client resp", "body", string(body))
}

func (s *E2ETestSuite) TestMoreBackendFromClient() {
	beServer1 := runTestServer("backend", s.T().Name()+"#backend:1", "--response", "PONG_1")
	defer beServer1.cancel()
	beServer2 := runTestServer("backend", s.T().Name()+"#backend:2", "--response", "PONG_2")
	defer beServer2.cancel()
	feServer1 := runTestServer("frontend", s.T().Name()+"#frontend:1")
	defer feServer1.cancel()

	runTestClient("client", s.T().Name()+"#client:1", feServer1.addr, "http://"+beServer1.addr+"/ping", "http://"+beServer2.addr+"/ping", "http://"+beServer2.addr+"/ping")

	time.Sleep(1 * time.Second)
}

func runTestServer(typeName string, instance string, args ...string) *TestServer {
	server := &TestServer{
		testServer: httptest.NewUnstartedServer(nil),
	}
	server.addr = server.testServer.Listener.Addr().String()
	started := make(chan struct{})
	runner := TestServerRunner(server.testServer, started)
	server.ctx, server.cancel = context.WithCancel(context.Background())
	go cmd.Execute(server.ctx, append([]string{typeName, "--listenaddr", server.addr, "--instance", invalidDomainNameRe.ReplaceAllString(instance, "-")}, args...), runner)
	<-started
	time.Sleep(1 * time.Second)

	return server
}

var invalidDomainNameRe = regexp.MustCompile(`[^a-zA-Z0-9.-]`)

func runTestClient(typeName string, instance string, addr string, args ...string) *TestClient {
	server := &TestClient{
		addr: addr,
	}
	server.ctx, server.cancel = context.WithCancel(context.Background())
	cmd.Execute(server.ctx, append([]string{typeName, "--server", addr, "--instance", invalidDomainNameRe.ReplaceAllString(instance, "-")}, args...), nil)

	return server
}
