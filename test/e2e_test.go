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
	"github.com/pgillich/opentracing-example/internal"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
	"github.com/pgillich/opentracing-example/internal/tracing"
	"github.com/stretchr/testify/suite"
)

const (
	jaegerUrlDefault = "http://localhost:14268/api/traces"
	otlpUrlDefault   = "http://localhost:4318"
)

var (
	// Optional configs
	_ = jaegerUrlDefault
	_ = otlpUrlDefault
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

type runTestServerType func(typeName string, instance string, config internal.ConfigSetter, args []string, newService model.NewService, log logr.Logger) *TestServer

func (s *E2ETestSuite) TestMoreBackendFromFrontend() {
	log := logger.GetLogger(s.T().Name())
	tracing.SetErrorHandlerLogger(&log)
	var runTestServer runTestServerType = runTestServerCmd

	beServer1 := runTestServer("backend", "backend-1", &internal.BackendConfig{}, []string{"PONG_1"}, internal.NewBackendService, log)
	defer beServer1.cancel()
	beServer2 := runTestServer("backend", "backend-2", &internal.BackendConfig{}, []string{"PONG_2"}, internal.NewBackendService, log)
	defer beServer2.cancel()
	feServer1 := runTestServer("frontend", "frontend", &internal.FrontendConfig{}, []string{}, internal.NewFrontendService, log)
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
	log := logger.GetLogger(s.T().Name())
	tracing.SetErrorHandlerLogger(&log)
	var runTestServer runTestServerType = runTestServerCmd

	beServer1 := runTestServer("backend", "backend-1", &internal.BackendConfig{}, []string{"PONG_1"}, internal.NewBackendService, log)
	defer beServer1.cancel()
	beServer2 := runTestServer("backend", "backend-2", &internal.BackendConfig{}, []string{"PONG_2"}, internal.NewBackendService, log)
	defer beServer2.cancel()
	feServer1 := runTestServer("frontend", "frontend", &internal.FrontendConfig{}, []string{}, internal.NewFrontendService, log)
	defer feServer1.cancel()

	runTestClient("client", "client-1", feServer1.addr, "http://"+beServer1.addr+"/ping", "http://"+beServer2.addr+"/ping", "http://"+beServer2.addr+"/ping")

	time.Sleep(1 * time.Second)
}

//nolint:deadcode,unused // old test
func runTestServerService(typeName string, instance string, config internal.ConfigSetter, args []string, newService model.NewService, log logr.Logger) *TestServer {
	server := &TestServer{
		testServer: httptest.NewUnstartedServer(nil),
	}
	server.addr = server.testServer.Listener.Addr().String()
	started := make(chan struct{})
	runner := TestServerRunner(server.testServer, started)
	server.ctx, server.cancel = context.WithCancel(context.Background())
	config.SetListenAddr(server.addr)
	config.SetInstance(invalidDomainNameRe.ReplaceAllString(instance, "-"))
	// Use only one URL
	//config.SetJaegerURL(jaegerUrlDefault)
	config.SetJaegerURL("-")
	config.SetOtlpURL(otlpUrlDefault)
	command := strings.Join(append(append([]string{typeName}, config.GetOptions()...), args...), " ")
	config.SetCommand(command)
	ctx := context.WithValue(server.ctx, model.CtxKeyCmd, command)
	ctx = context.WithValue(ctx, model.CtxKeyServerRunner, runner)
	go func() {
		if err := newService(ctx, config, log).Run(args); err != nil {
			log.Error(err, "RunService")
		}
	}()
	<-started
	//time.Sleep(1 * time.Second)

	return server
}

func runTestServerCmd(typeName string, instance string, config internal.ConfigSetter, args []string, newService model.NewService, log logr.Logger) *TestServer {
	server := &TestServer{
		testServer: httptest.NewUnstartedServer(nil),
	}
	server.addr = server.testServer.Listener.Addr().String()
	started := make(chan struct{})
	runner := TestServerRunner(server.testServer, started)
	server.ctx, server.cancel = context.WithCancel(context.Background())
	command := append([]string{
		typeName,
		"--listenaddr", server.addr,
		"--instance", invalidDomainNameRe.ReplaceAllString(instance, "-"),
		// Use only one URL
		"--jaegerURL", "-", // jaegerUrlDefault,
		"--otlpURL", otlpUrlDefault,
	}, args...)
	go func() {
		cmd.Execute(server.ctx, command, runner)
	}()
	<-started
	//time.Sleep(1 * time.Second)

	return server
}

var invalidDomainNameRe = regexp.MustCompile(`[^a-zA-Z0-9.-]`)

func runTestClient(typeName string, instance string, addr string, args ...string) *TestClient {
	server := &TestClient{
		addr: addr,
	}
	server.ctx, server.cancel = context.WithCancel(context.Background())
	cmd.Execute(server.ctx,
		append([]string{
			typeName,
			"--server", addr,
			"--instance", invalidDomainNameRe.ReplaceAllString(instance, "-"),
			// Use only one URL
			"--jaegerURL", "-", // jaegerUrlDefault,
			"--otlpURL", otlpUrlDefault,
		}, args...),
		nil,
	)

	return server
}
