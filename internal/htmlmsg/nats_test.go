package htmlmsg

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	natsserver "github.com/nats-io/nats-server/v2/test"
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

	o := natsserver.DefaultTestOptions
	o.Port = -1
	o.NoLog = false
	o.Debug = true
	o.Trace = true
	o.TraceVerbose = true
	natsSrv := natsserver.RunServer(&o)
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
