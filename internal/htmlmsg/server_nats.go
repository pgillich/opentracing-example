package htmlmsg

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/nats-io/nats.go"

	"github.com/pgillich/opentracing-example/internal/htmlmsg/model"
)

type NatsReqRespServer struct {
	url     string
	pattern string
	conn    *nats.Conn
	sub     *nats.Subscription
	log     logr.Logger
}

func NewNatsReqRespServer(natsURL string, pattern string, msgReciever model.MsgReceiver, log logr.Logger) (*NatsReqRespServer, error) {
	conn, err := nats.Connect(natsURL)
	if err != nil {
		return nil, err
	}

	sub, err := conn.Subscribe(pattern, func(msg *nats.Msg) {
		log = log.WithValues("Queue", msg.Subject, "Header", msg.Header, "Payload", string(msg.Data))
		log.WithValues("ReqMsg", msg).Info("nats.Conn.Subscribe")
		var err error //nolint:govet // hide above err
		ctx := context.Background()
		resp, err := msgReciever.Receive(ctx, model.Request{
			Queue:   msg.Subject,
			Header:  msg.Header,
			Payload: msg.Data,
		})
		if resp == nil {
			resp = &model.Response{Header: nats.Header{}}
		}
		if err != nil {
			resp.Error = err.Error()
		}
		if resp.Status == 0 {
			resp.Status = http.StatusInternalServerError
		}
		header := nats.Header(resp.Header)
		header.Add(NatsHeaderStatus, strconv.Itoa(resp.Status))
		header.Add(NatsHeaderError, resp.Error)
		log.WithValues("RespHeader", header, "RespPayload", resp.Payload).Info("nats.Msg.RespondMsg")
		if err = msg.RespondMsg(&nats.Msg{Header: header, Data: resp.Payload}); err != nil {
			log.Error(err, "nats.Msg.RespondMsg")
		}
	})
	if err != nil {
		log.Error(err, "nats.Conn.Subscribe")
	}

	return &NatsReqRespServer{
		url:     natsURL,
		pattern: pattern,
		conn:    conn,
		sub:     sub,
		log:     log,
	}, nil
}

func (s *NatsReqRespServer) Close() {
	s.sub.Unsubscribe() //nolint:errcheck,gosec // demo
	if err := s.conn.Drain(); err != nil {
		s.log.Error(err, "nats.Conn.Drain")
	}
}
