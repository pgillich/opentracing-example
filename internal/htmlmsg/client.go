package htmlmsg

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/pgillich/opentracing-example/internal/htmlmsg/model"
)

type HttpToMsg struct {
	DefaultTransport http.RoundTripper
	Client           model.MsgTransporter // NatsReqRespClient
}

func (h *HttpToMsg) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "queue" {
		return h.DefaultTransport.RoundTrip(req) //nolint:wrapcheck // demo
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Header.Add(model.QueueHeaderHost, req.URL.Hostname())
	req.Header.Add(model.QueueHeaderMethod, req.Method)
	hostname, _ := os.Hostname() //nolint:errcheck // demo
	req.Header.Add(model.QueueHeaderClient, hostname)
	queue := strings.TrimLeft(req.URL.Path, "/")
	resp, err := h.Client.MsgReqResp(req.Context(),
		model.Request{Queue: queue, Header: req.Header, Payload: reqBody},
	)
	if err != nil {
		return nil, err //nolint:wrapcheck // demo
	}

	return &http.Response{
		Request:       req,
		Proto:         "HTTP/1.0",
		ProtoMajor:    1,
		Header:        resp.Header,
		Close:         true,
		Body:          io.NopCloser(bytes.NewReader(resp.Payload)),
		ContentLength: int64(len(resp.Payload)),
		StatusCode:    resp.Status,
		Status:        fmt.Sprintf("%d %s", resp.Status, http.StatusText(resp.Status)),
	}, nil
}
