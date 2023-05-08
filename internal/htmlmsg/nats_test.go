package htmlmsg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	nats_server "github.com/nats-io/nats-server/v2/server"
	nats_test "github.com/nats-io/nats-server/v2/test"
	"github.com/stretchr/testify/suite"

	"github.com/pgillich/opentracing-example/internal/logger"
)

type NatsTestSuite struct {
	suite.Suite
	log logr.Logger
}

func TestNatsTestSuite(t *testing.T) {
	suite.Run(t, new(NatsTestSuite))
}

func (s *NatsTestSuite) SetupTest() {
	s.log = logger.GetLogger(s.T().Name())
}

func (s *NatsTestSuite) TestHttpRequest() {
	ctx := context.Background()

	o := nats_server.Options{
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 false,
		Debug:                 true,
		Trace:                 true,
		NoSigs:                true,
		MaxControlLine:        4096,
		DisableShortFirstPing: true,
	}
	natsSrv := NatsRunServerCallback(&o, nil)
	defer natsSrv.Shutdown()

	natsUrl := "nats://" + net.JoinHostPort(o.Host, strconv.Itoa(o.Port))

	srv, err := s.runServer(natsUrl)
	s.NoError(err)
	s.NotNil(srv)

	reqClient, err := NewNatsReqRespClient(natsUrl, s.log)
	s.NoError(err)

	httpClient := http.Client{
		Transport: &HttpToMsg{
			DefaultTransport: http.DefaultTransport,
			Client:           reqClient,
		},
	}

	payload := []byte("PING")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "queue://reqresp.ping", io.NopCloser(bytes.NewReader(payload)))
	s.NoError(err)
	httpReq.Header.Add("TestClient", s.T().Name())
	httpResp, err := httpClient.Do(httpReq)

	s.NoError(err)
	s.log.Info("test", "httpResp", httpResp)
	expHeader := http.Header{}
	expHeader.Add("RcvTestClient", s.T().Name())
	expBody := []byte("PONG")
	s.Equal(&http.Response{
		Request:       httpReq,
		Proto:         "HTTP/1.0",
		ProtoMajor:    1,
		Header:        expHeader,
		Close:         true,
		Body:          io.NopCloser(bytes.NewReader(expBody)),
		ContentLength: int64(len(expBody)),
		StatusCode:    http.StatusOK,
		Status:        fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)),
	}, httpResp)

	srv.Close()
}

func (s *NatsTestSuite) bindRoutes() http.Handler {
	r := chi.NewRouter()
	r.Post("/nats/reqresp.ping", func(w http.ResponseWriter, req *http.Request) {
		if req.Body == nil {
			w.WriteHeader(http.StatusBadRequest)

			return
		}
		defer req.Body.Close()
		body, _ := io.ReadAll(req.Body)
		s.log.WithValues("Header", req.Header, "Payload", string(body)).Info("Post /nats/reqresp.ping")
		w.Header().Add("RcvTestClient", req.Header.Get("TestClient"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PONG"))
	})

	return r
}

func (s *NatsTestSuite) runServer(natsUrl string) (*NatsReqRespServer, error) {
	msgToHttp := &MsgToHttp{
		Handler:    s.bindRoutes(),
		PathPrefix: "nats",
	}

	return NewNatsReqRespServer(natsUrl, "reqresp.*", msgToHttp, s.log)
}

// NatsRunServerCallback is an adapted github.com/nats-io/nats-server/v2/test/test.go:RunServerCallback
func NatsRunServerCallback(opts *nats_server.Options, callback func(*nats_server.Server)) *nats_server.Server {
	if opts == nil {
		opts = &nats_test.DefaultTestOptions
	}
	s, err := nats_server.NewServer(opts)
	if err != nil || s == nil {
		panic(fmt.Sprintf("No NATS Server object returned: %v", err))
	}

	if !opts.NoLog {
		s.ConfigureLogger()
	}

	if callback != nil {
		callback(s)
	}

	// Run server in Go routine.
	go s.Start()

	// Wait for accept loop(s) to be started
	if !s.ReadyForConnections(1 * time.Second) {
		panic("Unable to start NATS Server in Go Routine")
	}

	return s
}
