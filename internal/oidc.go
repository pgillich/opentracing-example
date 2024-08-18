package internal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"

	configs "github.com/pgillich/opentracing-example/configs"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/middleware"
	mw_client "github.com/pgillich/opentracing-example/internal/middleware/client"
	mw_server "github.com/pgillich/opentracing-example/internal/middleware/server"
	"github.com/pgillich/opentracing-example/internal/model"
	"github.com/pgillich/opentracing-example/internal/tracing"
)

const CookieStateLen = 16

type OidcConfig struct {
	ListenAddr         string
	Instance           string
	Command            string
	JaegerURL          string
	Oauth2ClientID     string
	Oauth2ClientSecret string
	OidcProviderURL    string
	OidcRedirectPath   string

	HttpClientCaptureMode configs.CaptureTransportMode
	HttpClientCaptureDir  string
}

func (c *OidcConfig) SetListenAddr(addr string) {
	c.ListenAddr = addr
}

func (c *OidcConfig) SetInstance(instance string) {
	c.Instance = instance
}

func (c *OidcConfig) SetJaegerURL(u string) {
	c.JaegerURL = u
}

func (c *OidcConfig) SetCommand(command string) {
	c.Command = command
}

func (c *OidcConfig) GetOptions() []string {
	return []string{"--listenaddr", c.ListenAddr, "--instance", c.Instance}
}

type Oidc struct {
	config       OidcConfig
	serverRunner model.ServerRunner
	log          *slog.Logger
	shutdown     <-chan struct{}
}

func NewOidcService(ctx context.Context, cfg interface{}) model.Service {
	_, log := logger.FromContext(ctx)
	if config, is := cfg.(*OidcConfig); !is {
		log.Error("config type", logger.KeyError, logger.ErrInvalidConfig)
		panic(logger.ErrInvalidConfig)
	} else if serverRunner, is := ctx.Value(model.CtxKeyServerRunner).(model.ServerRunner); !is {
		log.Error("server runner config", logger.KeyError, ErrInvalidServerRunner)
		panic(ErrInvalidServerRunner)
	} else {
		return &Oidc{
			config:       *config,
			serverRunner: serverRunner,
			log:          log.With("instance", config.Instance),
			shutdown:     ctx.Done(),
		}
	}
}

func (s *Oidc) Run(args []string) error {
	log := s.log
	ctx := logger.NewContext(context.Background(), log)
	log.With(
		logger.KeyCmd, s.config.Command,
		"config", s.config,
	).Info("Oidc start")

	traceExporter, err := tracing.JaegerProvider(s.config.JaegerURL)
	if err != nil {
		return err
	}
	if s.config.Instance == "-" {
		s.config.Instance, _ = os.Hostname() //nolint:errcheck // not important
	}
	tp := tracing.InitTracer(traceExporter, sdktrace.AlwaysSample(),
		"oidc.opentracing-example", s.config.Instance, s.config.Command, log,
	)
	defer func() {
		//nolint:govet // local err
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Error("error shutting down tracer provider", logger.KeyError, err)
		}
	}()

	httpClient := NewHttpClient(log, s.config.HttpClientCaptureMode, s.config.HttpClientCaptureDir)

	tr := tp.Tracer("github.com/pgillich/opentracing-example/oidc", trace.WithInstrumentationVersion(tracing.SemVersion()))
	traceState := trace.TraceState{}
	traceState, err = traceState.Insert(tracing.StateKeyOidcCommand, tracing.EncodeTracestateValue(s.config.Command))
	if err != nil {
		log.Error("unable to set command in state", logger.KeyError, err)
	} else {
		ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
			TraceState: traceState,
		}))
	}

	bag, err := tracing.NewBaggage(s.config.Instance, s.config.Command)
	if err != nil {
		return err
	}
	ctx = baggage.ContextWithBaggage(ctx, bag)

	// 	otel.SetTracerProvider(tp)
	spanKind := trace.SpanKindClient
	ctx, span := tr.Start(ctx, "Run "+s.config.Command,
		trace.WithAttributes(semconv.PeerServiceKey.String("ExampleOidcService")),
		trace.WithSpanKind(spanKind),
	)
	ctx, log = logger.FromContext(ctx, "traceID", span.SpanContext().TraceID().String(), "spanID", span.SpanContext().SpanID().String())
	log.With("spanKind", spanKind).Info("SPAN_START")
	defer func() {
		spanText, _ := span.SpanContext().MarshalJSON() //nolint:errcheck // not important
		log.With(
			"span", string(spanText),
		).Info("SPAN_END")
		span.End()
		tp.ForceFlush(context.Background()) //nolint:errcheck,gosec // not important
	}()

	return s.run(ctx, httpClient)
}

func NewHttpClient(log *slog.Logger, captureTransportMode configs.CaptureTransportMode, httpClientCaptureDir string) *http.Client {
	httpClient := &http.Client{Transport: otelhttp.NewTransport(
		mw_client.NewMetricTransport(
			mw_client.NewLogTransport(
				mw_client.NewCaptureTransport(
					http.DefaultTransport,
					captureTransportMode,
					httpClientCaptureDir,
					[]mw_client.CaptureMatcher{
						mw_client.CaptureEqualRequestURL(),
						mw_client.CaptureReMatcherRequestURL(),
					},
				),
				slog.LevelInfo,
				slog.LevelInfo,
			),
			middleware.GetMeter(log),
			"http_out", "HTTP out response", map[string]string{
				"service":        "oidc",
				"target_service": "frontend",
			},
			middleware.FirstErr,
		),
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		otelhttp.WithSpanOptions(trace.WithAttributes(
			attribute.String(tracing.SpanKeyComponent, tracing.SpanKeyComponentValue),
		)),
	)}
	return httpClient
}

func (s *Oidc) run(ctx context.Context, httpClient *http.Client) error {
	_, log := logger.FromContext(ctx)

	var h http.Handler
	hostname, _ := os.Hostname() //nolint:errcheck // not important
	if s.config.Instance == "-" {
		s.config.Instance = hostname
	}

	traceExporter, err := tracing.JaegerProvider(s.config.JaegerURL)
	if err != nil {
		return err
	}
	tp := tracing.InitTracer(traceExporter, sdktrace.AlwaysSample(),
		"oidc.opentracing-example", s.config.Instance, "", log,
	)
	defer func() { //nolint:contextcheck // Break context
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Error("error shutting down tracer provider", logger.KeyError, err)
		}
	}()
	tr := tp.Tracer(
		"github.com/pgillich/opentracing-example/oidc",
		trace.WithInstrumentationVersion(tracing.SemVersion()),
	)

	// CHI

	r := chi.NewRouter()
	r.Use(chi_middleware.Recoverer)
	r.Use(mw_server.ChiLoggerBaseMiddleware(log))
	r.Use(mw_server.ChiTracerMiddleware(tr, s.config.Instance))
	r.Use(mw_server.ChiLoggerMiddleware(slog.LevelInfo, slog.LevelInfo))
	r.Use(mw_server.ChiMetricMiddleware(middleware.GetMeter(log),
		"http_in", "HTTP in response", map[string]string{
			"service": "oidc",
		}, log,
	))
	if err := s.setOidcRoutes(ctx, r, httpClient); err != nil {
		log.Error("unable to set oidc routes", logger.KeyError, err)
		return err
	}
	h = r

	s.serverRunner(h, s.shutdown, s.config.ListenAddr, log)
	log.Info("oidc started")

	return nil
}

func (s *Oidc) setOidcRoutes(ctx context.Context, r *chi.Mux, httpClient *http.Client) error {
	_, log := logger.FromContext(ctx)
	ctx = oidc.ClientContext(ctx, httpClient)
	oidcProvider, err := oidc.NewProvider(ctx, s.config.OidcProviderURL)
	if err != nil {
		log.Error("unable to get provider:", logger.KeyError, err)
		return err
	}
	oidcRedirectURL, err := url.JoinPath(s.serverURL(), s.config.OidcRedirectPath)
	if err != nil {
		log.Error("unable to get RedirectURL:", logger.KeyError, err)
		return err
	}
	oauth2Config := oauth2.Config{
		ClientID:     s.config.Oauth2ClientID,
		ClientSecret: s.config.Oauth2ClientSecret,
		Endpoint:     oidcProvider.Endpoint(),
		RedirectURL:  oidcRedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		state, err := randString(CookieStateLen)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)

			return
		}
		setCallbackCookie(w, r, "state", state)

		http.Redirect(w, r, oauth2Config.AuthCodeURL(state), http.StatusFound)
	})

	r.Get(s.config.OidcRedirectPath, func(w http.ResponseWriter, r *http.Request) {
		state, err := r.Cookie("state")
		if err != nil {
			http.Error(w, "state not found", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("state") != state.Value {
			http.Error(w, "state did not match", http.StatusBadRequest)
			return
		}

		oauth2Token, err := oauth2Config.Exchange(ctx, r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		userInfo, err := oidcProvider.UserInfo(ctx, oauth2.StaticTokenSource(oauth2Token))
		if err != nil {
			http.Error(w, "Failed to get userinfo: "+err.Error(), http.StatusInternalServerError)
			return
		}

		resp := struct {
			OAuth2Token *oauth2.Token
			UserInfo    *oidc.UserInfo
		}{oauth2Token, userInfo}
		data, err := json.MarshalIndent(resp, "", "    ") //nolint:musttag // Not important
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Info("oidc resp", "body", string(data))
		if _, err := w.Write(data); err != nil {
			log.Error("unable to send HTTP response", logger.KeyError, err)
		}
	})

	return nil
}

func (s *Oidc) serverURL() string {
	return "http://" + s.config.ListenAddr
}

func randString(nByte int) (string, error) {
	b := make([]byte, nByte)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func setCallbackCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   int(time.Hour.Seconds()),
		Secure:   r.TLS != nil,
		HttpOnly: true,
	}
	http.SetCookie(w, c)
}
