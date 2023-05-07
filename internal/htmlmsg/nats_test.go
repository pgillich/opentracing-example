package htmlmsg

import (
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
	"github.com/google/uuid"
	nats_server "github.com/nats-io/nats-server/v2/server"
	nats_test "github.com/nats-io/nats-server/v2/test"
	"github.com/stretchr/testify/suite"

	"github.com/pgillich/opentracing-example/internal/htmlmsg/model"
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

func (s *NatsTestSuite) TestRequest() {
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

	msgID := uuid.NewString()
	header := http.Header{}
	payload := []byte("PING")
	req := model.Request{Queue: "reqresp.ping", MsgID: msgID, Header: header, Payload: payload}
	resp, err := reqClient.Request(ctx, req)

	s.NoError(err)
	s.log.Info("test", "resp", resp)
	expHeader := http.Header{}
	expHeader.Add("TestHeader", "T")
	s.Equal(&model.Response{
		Header:  expHeader,
		Payload: []byte("PONG"),
		Status:  http.StatusOK,
		Error:   "",
	}, resp)

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
		w.Header().Add("TestHeader", "T")
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
