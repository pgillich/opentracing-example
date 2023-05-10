package htmlmsg

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"

	"net/http/httptest"

	"github.com/pgillich/opentracing-example/internal/htmlmsg/model"
)

var _ model.MsgReceiver = (*MsgToHttp)(nil)

type MsgToHttp struct {
	Handler    http.Handler
	PathPrefix string
}

func (h *MsgToHttp) Receive(ctx context.Context,
	req model.Request,
) (*model.Response, error) {
	respRec := &httptest.ResponseRecorder{Body: &bytes.Buffer{}}
	header := http.Header(req.Header)
	path, _ := url.JoinPath("/", h.PathPrefix, req.Queue) //nolint:errcheck // demo
	reqUrl := url.URL{
		Scheme: "queue",
		Host:   header.Get(model.QueueHeaderHost),
		Path:   path,
	}
	httpReq, err := http.NewRequestWithContext(ctx,
		header.Get(model.QueueHeaderMethod),
		reqUrl.String(),
		io.NopCloser(bytes.NewReader(req.Payload)),
	)
	if err != nil {
		return nil, err
	}
	httpReq.Header = req.Header

	h.Handler.ServeHTTP(respRec, httpReq)

	hostname, _ := os.Hostname() //nolint:errcheck // demo
	respRec.Header().Add(model.QueueHeaderServer, hostname)

	return &model.Response{
		Header:  respRec.Header(),
		Status:  respRec.Code,
		Payload: respRec.Body.Bytes(),
	}, nil
}
