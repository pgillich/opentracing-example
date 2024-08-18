package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	yaml "github.com/goccy/go-yaml"

	configs "github.com/pgillich/opentracing-example/configs"
	"github.com/pgillich/opentracing-example/internal/logger"
)

var ErrCapNotMatch = errors.New("capture not match")
var ErrCapResponse = errors.New("capture response error")

type DelayedReaderPersister struct {
	body     io.ReadCloser
	payload  []byte
	closed   chan struct{}
	isNil    bool
	isNilish bool
	mu       sync.Mutex
}

func NewDelayedReaderPersister(body io.ReadCloser) *DelayedReaderPersister {
	drp := &DelayedReaderPersister{
		body:     body,
		closed:   make(chan struct{}),
		isNil:    body == nil,
		isNilish: IsNilish(body),
		mu:       sync.Mutex{},
	}
	if drp.isNilish {
		close(drp.closed)
	}
	return drp
}

func (drp *DelayedReaderPersister) Close() error {
	drp.mu.Lock()
	defer drp.mu.Unlock()
	select {
	case <-drp.closed:
		return nil
	default:
		close(drp.closed)
	}
	return drp.body.Close() //nolint:wrapcheck // wrapper
}

func (drp *DelayedReaderPersister) Done() <-chan struct{} {
	return drp.closed
}

func (drp *DelayedReaderPersister) Read(p []byte) (n int, err error) { //nolint:nonamedreturns // orig def
	if drp.isNilish {
		return 0, io.EOF
	}
	n, err = drp.body.Read(p)
	drp.mu.Lock()
	defer drp.mu.Unlock()
	drp.payload = append(drp.payload, p[:n]...)

	return n, err //nolint:wrapcheck // wrapper
}

func (drp *DelayedReaderPersister) Payload() []byte {
	return drp.payload
}

func (drp *DelayedReaderPersister) IsNil() bool {
	return drp.isNil
}

type Request struct {
	Method           string
	URL              string
	Proto            string
	ProtoMajor       int
	ProtoMinor       int
	Header           http.Header
	Body             *string
	ContentLength    int64
	TransferEncoding []string
	Close            bool
	Host             string
	Form             url.Values
	PostForm         url.Values
	MultipartForm    *multipart.Form
	Trailer          http.Header
	RemoteAddr       string
	RequestURI       string
	//TLS              *tls.ConnectionState
	TestTimestamp time.Time `yaml:"testTimestamp"`
}

func NewRequest(r0 *http.Request) Request {
	r := r0.Clone(context.Background())
	return Request{
		Method:           r.Method,
		URL:              safeUrl(r.URL).String(),
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           r.Header,
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Host:             r.Host,
		Form:             r.Form,
		PostForm:         r.PostForm,
		MultipartForm:    r.MultipartForm,
		Trailer:          r.Trailer,
		RemoteAddr:       r.RemoteAddr,
		RequestURI:       r.RequestURI,
		//TLS:              r.TLS,
		TestTimestamp: time.Now(),
	}
}

type Response struct {
	Status           string
	StatusCode       int
	Proto            string
	ProtoMajor       int
	ProtoMinor       int
	Header           http.Header
	Body             *string
	ContentLength    int64
	TransferEncoding []string
	Close            bool
	Uncompressed     bool
	Trailer          http.Header
	//TLS              *tls.ConnectionState
	TestTimestamp time.Time `yaml:"testTimestamp"`
}

func NewResponse(r *http.Response) Response {
	return Response{
		Status:           r.Status,
		StatusCode:       r.StatusCode,
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           r.Header.Clone(),
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Uncompressed:     r.Uncompressed,
		Trailer:          r.Trailer.Clone(),
		//TLS:              r.TLS,
		TestTimestamp: time.Now(),
	}
}

func (r Response) ToHttpResponse() *http.Response {
	return &http.Response{
		Status:           r.Status,
		StatusCode:       r.StatusCode,
		Proto:            r.Proto,
		ProtoMajor:       r.ProtoMajor,
		ProtoMinor:       r.ProtoMinor,
		Header:           r.Header.Clone(),
		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,
		Close:            r.Close,
		Uncompressed:     r.Uncompressed,
		Trailer:          r.Trailer.Clone(),
	}
}

type CaptureItem struct {
	Request     Request `yaml:"request"`
	reqPayload  *DelayedReaderPersister
	Response    Response `yaml:"response"`
	respPayload *DelayedReaderPersister
}

type CaptureMatcher func(*http.Request, *CaptureItem) bool

type CaptureTransport struct {
	rt          http.RoundTripper
	mode        configs.CaptureTransportMode
	dirPath     string
	capMatchers []CaptureMatcher
}

func NewCaptureTransport(base http.RoundTripper, mode configs.CaptureTransportMode, dirPath string, capMatchers []CaptureMatcher) *CaptureTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &CaptureTransport{
		rt:          base,
		mode:        mode,
		dirPath:     dirPath,
		capMatchers: capMatchers,
	}
}

var fakeDirPathTemplate, _ = template.New("FakeDirPath").Parse("{{.Host}}/{{.Url.Scheme}}") //nolint:errcheck,gochecknoglobals // Just const
var replaceFakeDirPathChars = map[string]*regexp.Regexp{                                    //nolint:gochecknoglobals // Just const
	"-": regexp.MustCompile("[^a-zA-Z0-9_=/.]"),
}

const fakeDirPathPerm = 0o750

var fakeFileNameTemplate, _ = template.New("FakeFileName").Parse("{{.Request.Method}}_{{.Url.Path}}.yaml") //nolint:errcheck,gochecknoglobals // Just const
var disabledFakeFileNameChars = map[string]*regexp.Regexp{                                                 //nolint:gochecknoglobals // Just const
	"%": regexp.MustCompile("/"),
	"-": regexp.MustCompile("[^a-zA-Z0-9_=/&%.]"),
}

const fakeFileNamePerm = 0o640

func renderFakePath(tmpl *template.Template, disabledChars map[string]*regexp.Regexp, r *http.Request) string {
	var fakePath string
	u := r.URL
	if u == nil {
		u = &url.URL{}
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	rendered := &strings.Builder{}
	err := tmpl.Execute(rendered, map[string]any{
		"Request": r,
		"Host":    host,
		"Url":     u,
	})
	if err != nil {
		fakePath = err.Error()
	} else {
		fakePath = rendered.String()
	}

	for repl, pat := range disabledChars {
		fakePath = pat.ReplaceAllLiteralString(fakePath, repl)
	}

	return fakePath
}

func (t *CaptureTransport) createCaptureFile(r *http.Request) (string, error) {
	fakeDirPath, captureFilePath := t.getCaptureFilePaths(r)
	if err := os.MkdirAll(fakeDirPath, fakeDirPathPerm); err != nil {
		return "", err
	}
	if err := TouchFile(captureFilePath, fakeFileNamePerm); err != nil {
		return "", err
	}

	return captureFilePath, nil
}

func (t *CaptureTransport) getCaptureFilePaths(r *http.Request) (string, string) {
	fakeDirPath := path.Join(t.dirPath, renderFakePath(fakeDirPathTemplate, replaceFakeDirPathChars, r))
	captureFilePath := path.Join(fakeDirPath, renderFakePath(fakeFileNameTemplate, disabledFakeFileNameChars, r))
	return fakeDirPath, captureFilePath
}

func (t *CaptureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var res *http.Response
	var rtErr error

	switch t.mode {
	case configs.CaptureTransportModeNone:
		res, rtErr = t.rt.RoundTrip(r)
	case configs.CaptureTransportModeRecord:
		captureFilePath, err := t.createCaptureFile(r)
		_, log := logger.FromContext(r.Context(), "method", r.Method, "url", safeUrl(r.URL), "file", captureFilePath)
		if err != nil {
			log.Error("unable to create capture file", logger.KeyError, err)
			res, rtErr = t.rt.RoundTrip(r)

			break
		}
		res, rtErr = t.captureHttpClient(r, captureFilePath) //nolint:contextcheck // r contains
	case configs.CaptureTransportModeFake:
		_, captureFilePath := t.getCaptureFilePaths(r)
		_, log := logger.FromContext(r.Context(), "method", r.Method, "url", safeUrl(r.URL), "file", captureFilePath)
		var err error
		res, err = t.fakeHttpResponse(r, captureFilePath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				log.Info("capture file missing, skip faking")
			} else {
				log.Info("unable to read capture, skip faking")
			}
			res, rtErr = t.rt.RoundTrip(r)
		}
	}

	return res, rtErr //nolint:wrapcheck // It's a decorator func
}

func CaptureReMatcherRequestURL() CaptureMatcher {
	return func(r *http.Request, ci *CaptureItem) bool {
		_, log := logger.FromContext(r.Context(), "method", r.Method, "url", safeUrl(r.URL))

		rURL := safeUrl(r.URL).String()
		pURL := ci.Request.URL
		if match, err := regexp.MatchString(pURL, rURL); err != nil {
			log.Warn("wrong URL pattern", "pattern", pURL)
		} else if match {
			return true
		}
		return false
	}
}

func CaptureEqualRequestURL() CaptureMatcher {
	return func(r *http.Request, ci *CaptureItem) bool {
		return safeUrl(r.URL).String() == ci.Request.URL
	}
}

func (t *CaptureTransport) captureHttpClient(r *http.Request, captureFilePath string) (*http.Response, error) {
	//_, log := logger.FromContext(r.Context(), "method", r.Method, "url", safeUrl(r.URL), "file", captureFilePath)
	var res *http.Response
	var rtErr error
	capItem := CaptureItem{
		Request:    NewRequest(r),
		reqPayload: NewDelayedReaderPersister(r.Body),
	}
	capItem.reqPayload = NewDelayedReaderPersister(r.Body)
	r.Body = capItem.reqPayload
	// log.Info("TLS", "req_tls", r.TLS)

	res, rtErr = t.rt.RoundTrip(r)

	// log.Info("TLS", "res_tls", res.TLS)
	capItem.Response = NewResponse(res)
	capItem.respPayload = NewDelayedReaderPersister(res.Body)
	res.Body = capItem.respPayload

	go t.saveCapture(r, res, capItem, captureFilePath)

	return res, rtErr //nolint:wrapcheck // It's a decorator func
}

func (*CaptureTransport) saveCapture(r *http.Request, res *http.Response, capItem CaptureItem, captureFilePath string) {
	_, log := logger.FromContext(r.Context(), "method", r.Method, "url", safeUrl(r.URL), "file", captureFilePath)

	select {
	case <-capItem.reqPayload.Done():
	case <-r.Context().Done():
		log.Warn("ctx done, req body was not closed")
		_ = capItem.reqPayload.Close() //nolint:errcheck // Not important
	}
	select {
	case <-capItem.respPayload.Done():
	case <-r.Context().Done():
		log.Warn("ctx done, resp body was not closed")
		_ = capItem.respPayload.Close() //nolint:errcheck // Not important
	}
	if !capItem.reqPayload.IsNil() {
		payload := string(capItem.reqPayload.Payload())
		capItem.Request.Body = &payload
	}
	if !capItem.respPayload.IsNil() {
		payload := string(capItem.respPayload.Payload())
		capItem.Response.Body = &payload
	}

	captureTitle := fmt.Sprintf("%s %s %s", r.Method, capItem.Request.URL, res.Status)
	yamlDoc, err := yaml.MarshalWithOptions(&capItem,
		yaml.CustomMarshaler[time.Time](func(t time.Time) ([]byte, error) {
			return []byte(t.Format(time.DateTime)), nil
		}),
		yaml.UseLiteralStyleIfMultiline(true),
		yaml.WithComment(yaml.CommentMap{"$": []*yaml.Comment{yaml.HeadComment(captureTitle, time.Now().Format(time.DateTime))}}),
	)
	if err != nil {
		log.Error("unable to marshal captured HTTP", logger.KeyError, err)
	} else {
		yamlDoc = append([]byte("---\n"), yamlDoc...)
		yamlDoc = append(yamlDoc, '\n')
		if err = AppendFile(captureFilePath, yamlDoc, fakeFileNamePerm); err != nil {
			log.Error("unable to write captured HTTP", logger.KeyError, err)
		} else {
			log.Info("capture doc written", "title", captureTitle)
		}
	}
}

func (t *CaptureTransport) fakeHttpResponse(r *http.Request, captureFilePath string) (*http.Response, error) {
	_, log := logger.FromContext(r.Context(), "method", r.Method, "url", safeUrl(r.URL), "file", captureFilePath)
	yamlFile, err := os.Open(captureFilePath) //nolint:gosec // Don't worry about filename in variable
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bufio.NewReader(yamlFile))

	for err == nil {
		capItem := &CaptureItem{}
		err = decoder.Decode(capItem)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Warn("unable to unmarshall yaml")
				return nil, err
			}
			return nil, ErrCapNotMatch
		}

		for _, matcher := range t.capMatchers {
			if matcher(r, capItem) {
				body := ""
				if capItem.Response.Body != nil {
					body = *capItem.Response.Body
				}
				log.Debug("using fake response", "status", capItem.Response.StatusCode, "body", body)
				return fakeResponse(r, capItem)
			}
		}
	}

	return nil, ErrCapNotMatch
}

func fakeResponse(r *http.Request, capItem *CaptureItem) (*http.Response, error) {
	res := capItem.Response.ToHttpResponse()
	res.Request = r
	r.Response = res
	if capItem.Response.Body != nil {
		res.Body = io.NopCloser(strings.NewReader(*capItem.Response.Body))
	}
	return res, nil
}

func TouchFile(name string, perm os.FileMode) error {
	file, err := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, perm) //nolint:gosec // perm is defined as const
	if err != nil {
		return err
	}
	return file.Close()
}

func AppendFile(name string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, perm) //nolint:gosec // perm is defined as const
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err1 := f.Close(); err1 != nil && err == nil {
		err = err1
	}

	return err
}

func IsNilish(val any) bool {
	if val == nil {
		return true
	}

	v := reflect.ValueOf(val)
	k := v.Kind()
	switch k { //nolint:exhaustive // Only nilable is important
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer,
		reflect.UnsafePointer, reflect.Interface /*, reflect.Slice*/ : // Zero slice does not cause panic
		return v.IsNil()
	}

	return false
}

func safeUrl(u *url.URL) *url.URL {
	if u == nil {
		u = &url.URL{}
	}
	return u
}
