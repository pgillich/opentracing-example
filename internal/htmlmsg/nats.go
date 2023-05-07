package htmlmsg

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/nats-io/nats.go"

	"github.com/pgillich/opentracing-example/internal/htmlmsg/model"
)

const NatsHeaderMsgID = "X-Nats-MsgID"
const NatsHeaderStatus = "X-Nats-Status"
const NatsHeaderError = "X-Nats-Error"

var _ model.MsgRequester = (*NatsReqRespClient)(nil)

type NatsReqRespClient struct {
	url  string
	conn *nats.Conn
	log  logr.Logger
}

func NewNatsReqRespClient(natsUrl string, log logr.Logger) (*NatsReqRespClient, error) {
	conn, err := nats.Connect(natsUrl)
	if err != nil {
		return nil, err
	}

	return &NatsReqRespClient{
		url:  natsUrl,
		conn: conn,
		log:  log,
	}, nil
}

func (c *NatsReqRespClient) Request(ctx context.Context, req model.Request) (*model.Response, error) {
	log := c.log.WithValues("Queue", req.Queue, "MsgID", req.MsgID, "Header", req.Header, "Payload", string(req.Payload))
	header := nats.Header(req.Header)
	header.Add(NatsHeaderMsgID, req.MsgID)

	respMsg, err := c.conn.RequestMsgWithContext(ctx, &nats.Msg{
		Subject: req.Queue,
		Header:  header,
		Data:    req.Payload,
	})

	if err != nil {
		log.Error(err, "nats.Conn.Request")

		return nil, err
	}
	log.WithValues("ReqMsg", respMsg).Info("nats.Conn.Request")
	msgId := respMsg.Header.Get(NatsHeaderMsgID)
	respMsg.Header.Del(NatsHeaderMsgID)
	status, _ := strconv.Atoi(respMsg.Header.Get(NatsHeaderStatus)) //nolint:errcheck // demo
	respMsg.Header.Del(NatsHeaderStatus)
	errTxt := respMsg.Header.Get(NatsHeaderError)
	respMsg.Header.Del(NatsHeaderError)
	resp := &model.Response{
		Header:  respMsg.Header,
		Payload: respMsg.Data,
		Status:  status,
		Error:   errTxt,
	}
	if msgId != req.MsgID {
		return resp, fmt.Errorf("inconsistent MsgID: %s, %s", msgId, req.MsgID)
	}

	return resp, nil
}

func (c *NatsReqRespClient) Close() {
	if err := c.conn.Drain(); err != nil {
		c.log.Error(err, "nats.Conn.Drain")
	}
}

type NatsReqRespServer struct {
	url     string
	pattern string
	conn    *nats.Conn
	sub     *nats.Subscription
	log     logr.Logger
}

func NewNatsReqRespServer(natsUrl string, pattern string, msgReciever model.MsgReceiver, log logr.Logger) (*NatsReqRespServer, error) {
	conn, err := nats.Connect(natsUrl)
	if err != nil {
		return nil, err
	}

	sub, err := conn.Subscribe(pattern, func(msg *nats.Msg) {
		var err error //nolint:govet // hide above err
		ctx := context.Background()
		msgID := msg.Header.Get(NatsHeaderMsgID)
		resp, err := msgReciever.Receive(ctx, model.Request{
			Queue:   msg.Subject,
			MsgID:   msgID,
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
		header.Add(NatsHeaderMsgID, msgID)
		header.Add(NatsHeaderStatus, strconv.Itoa(resp.Status))
		header.Add(NatsHeaderError, resp.Error)
		if err = msg.RespondMsg(&nats.Msg{Header: header, Data: resp.Payload}); err != nil {
			log.Error(err, "nats.Msg.RespondMsg")
		}
	})
	if err != nil {
		log.Error(err, "nats.Conn.Subscribe")
	}

	return &NatsReqRespServer{
		url:     natsUrl,
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
