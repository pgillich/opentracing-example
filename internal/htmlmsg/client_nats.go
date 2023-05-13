package htmlmsg

import (
	"context"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/nats-io/nats.go"

	"github.com/pgillich/opentracing-example/internal/htmlmsg/model"
)

var _ model.MsgTransporter = (*NatsReqRespClient)(nil)

type NatsReqRespClient struct {
	url  string
	conn *nats.Conn
	log  logr.Logger
}

func NewNatsReqRespClient(natsURL string, log logr.Logger) (*NatsReqRespClient, error) {
	conn, err := nats.Connect(natsURL)
	if err != nil {
		return nil, err
	}

	return &NatsReqRespClient{
		url:  natsURL,
		conn: conn,
		log:  log,
	}, nil
}

func (c *NatsReqRespClient) MsgReqResp(ctx context.Context, req model.Request) (*model.Response, error) {
	log := c.log.WithValues("Queue", req.Queue, "Header", req.Header, "Payload", string(req.Payload))
	header := nats.Header(req.Header)

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

	return resp, nil
}

func (c *NatsReqRespClient) Close() {
	if err := c.conn.Drain(); err != nil {
		c.log.Error(err, "nats.Conn.Drain")
	}
}
