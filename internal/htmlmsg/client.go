package htmlmsg

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/pgillich/opentracing-example/internal/htmlmsg/model"
)

type HttpToMsg struct {
	Client model.MsgRequester // NatsReqRespClient
}

func (h *HttpToMsg) RoundTrip(req *http.Request) (*http.Response, error) {
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	msgID := uuid.NewString()
	resp, err := h.Client.Request(req.Context(),
		model.Request{Queue: req.URL.Hostname(), MsgID: msgID, Header: req.Header, Payload: reqBody},
	)
	if err != nil {
		return nil, err //nolint:wrapcheck // demo
	}

	return &http.Response{
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		Header:     resp.Header,
		Close:      true,
		Body:       io.NopCloser(bytes.NewReader(resp.Payload)),
		StatusCode: resp.Status,
		Status:     fmt.Sprintf("%d %s", resp.Status, http.StatusText(resp.Status)),
	}, nil
}
