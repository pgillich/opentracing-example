package internal

import (
	"context"
	"io"
	"net/http"
	"strings"

	"emperror.dev/errors"
	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
	"github.com/pgillich/opentracing-example/internal/tracing"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type FrontendConfig struct {
	ListenAddr string
	Instance   string
	Command    string
	JaegerURL  string
}

func (c *FrontendConfig) SetListenAddr(addr string) {
	c.ListenAddr = addr
}

func (c *FrontendConfig) SetInstance(instance string) {
	c.Instance = instance
}

func (c *FrontendConfig) SetJaegerURL(url string) {
	c.JaegerURL = url
}

func (c *FrontendConfig) SetCommand(command string) {
	c.Command = command
}

func (c *FrontendConfig) GetOptions() []string {
	return []string{"--listenaddr", c.ListenAddr, "--instance", c.Instance}
}

type Frontend struct {
	config       FrontendConfig
	serverRunner model.ServerRunner
	log          logr.Logger
	shutdown     <-chan struct{}
}

func NewFrontendService(ctx context.Context, cfg interface{}, log logr.Logger) model.Service {
	if config, is := cfg.(*FrontendConfig); !is {
		log.Error(logger.ErrInvalidConfig, "config type")
		panic(logger.ErrInvalidConfig)
	} else if serverRunner, is := ctx.Value(model.CtxKeyServerRunner).(model.ServerRunner); !is {
		log.Error(ErrInvalidServerRunner, "server runner config")
		panic(ErrInvalidServerRunner)
	} else {
		return &Frontend{
			config:       *config,
			serverRunner: serverRunner,
			log:          log,
			shutdown:     ctx.Done(),
		}
	}
}

func (s *Frontend) Run(args []string) error {
	s.log = s.log.WithValues("args", args)
	var h http.Handler
	s.log.WithValues("config", s.config).Info("Frontend start")

	traceExporter, err := tracing.JaegerProvider(s.config.JaegerURL)
	if err != nil {
		return err
	}
	tp := tracing.InitTracer(traceExporter, sdktrace.AlwaysSample(),
		s.config.Instance, s.config.Instance, "", s.log,
	)
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			s.log.Error(err, "Error shutting down tracer provider")
		}
	}()

	// CHI

	r := chi.NewRouter()
	r.Use(chi_middleware.RequestLogger(&logger.ChiLogr{Logger: s.log}))
	r.Use(chi_middleware.Recoverer)

	r.Get("/proxy", func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			s.writeErr(w, http.StatusInternalServerError, errors.New("empty response"))

			return
		}
		defer r.Body.Close() //nolint:errcheck // not important
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, err)

			return
		}

		ctx, span := chiSpan(tp, "github.com/pgillich/opentracing-example/frontend", "/proxy", s.config.Instance, r, s.log)
		defer func() {
			spanText, _ := span.SpanContext().MarshalJSON() //nolint:errcheck // not important
			s.log.WithValues(
				"service", "frontend",
				"span", string(spanText),
			).Info("Span END")
			span.End()
			tp.ForceFlush(context.Background()) //nolint:errcheck,gosec // not important
		}()

		bodies := []string{}
		for _, beURL := range strings.Split(string(body), " ") {
			body, err := s.sendToBackend(ctx, beURL) //nolint:govet // err shadow
			if err != nil {
				s.writeErr(w, http.StatusInternalServerError, err)

				return
			}
			bodies = append(bodies, body)
		}

		if _, err = w.Write([]byte(strings.Join(bodies, " "))); err != nil {
			s.log.Error(err, "unable to write response")
		}
	})
	h = r

	s.serverRunner(h, s.shutdown, s.config.ListenAddr, s.log)
	s.log.Info("Frontend exit")

	return nil
}

func (s *Frontend) sendToBackend(ctx context.Context, beURL string) (string, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, beURL, http.NoBody,
	)
	if err != nil {
		return "", errors.Wrap(err, "unable to send request")
	}
	httpClient := &http.Client{Transport: otelhttp.NewTransport(
		http.DefaultTransport,
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
	)}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "unable to send request")
	}
	if resp.Body == nil {
		return "", errors.New("empty body")
	}
	beBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "unable to read response")
	}
	resp.Body.Close() //nolint:errcheck,gosec // not important

	return string(beBody), nil
}

/*
		// ECHO

	e := echo.New()
	e.Use(logger.EchoLogr(s.log))
	e.Use(echo_middleware.Recover())
	e.GET("/proxy", func(c echo.Context) error {
		if c.Request().Body == nil {
			return c.String(http.StatusInternalServerError, "empty response") //nolint:wrapcheck // Echo
		}
		defer c.Request().Body.Close() //nolint:errcheck // not important
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error()) //nolint:wrapcheck // Echo
		}
		bodies := []string{}
		for _, beURL := range strings.Split(string(body), " ") {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, beURL, http.NoBody)
			if err != nil {
				return c.String(http.StatusInternalServerError, err.Error()) //nolint:wrapcheck // Echo
			}
			httpClient := &http.Client{}
			resp, err := httpClient.Do(req)
			if err != nil {
				return c.String(http.StatusInternalServerError, err.Error()) //nolint:wrapcheck // Echo
			}
			if resp.Body == nil {
				return c.String(http.StatusInternalServerError, "empty response") //nolint:wrapcheck // Echo
			}
			beBody, err := io.ReadAll(resp.Body)
			resp.Body.Close() //nolint:errcheck,gosec // not important
			if err != nil {
				return c.String(http.StatusInternalServerError, err.Error()) //nolint:wrapcheck // Echo
			}
			bodies = append(bodies, string(beBody))
		}

		return c.String(http.StatusOK, strings.Join(bodies, " ")) //nolint:wrapcheck // Echo
	})
	h = e
*/

/*
	// GIN

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(ginlogr.Ginlogr(s.log, time.RFC3339, false))
	router.Use(ginlogr.RecoveryWithLogr(s.log, time.RFC3339, false, true))
	router.GET("/proxy", func(c *gin.Context) {
		if c.Request.Body == nil {
			c.String(http.StatusInternalServerError, "empty response")
			return //nolint:nlreturn // no problem
		}
		defer c.Request.Body.Close() //nolint:errcheck // not important
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return //nolint:nlreturn // no problem
		}
		bodies := []string{}
		for _, beURL := range strings.Split(string(body), " ") {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, beURL, http.NoBody)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return //nolint:nlreturn // no problem
			}
			httpClient := &http.Client{}
			resp, err := httpClient.Do(req)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return //nolint:nlreturn // no problem
			}
			if resp.Body == nil {
				c.String(http.StatusInternalServerError, "empty response")
				return //nolint:nlreturn // no problem
			}
			beBody, err := io.ReadAll(resp.Body)
			resp.Body.Close() //nolint:errcheck,gosec // not important
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return //nolint:nlreturn // no problem
			}
			bodies = append(bodies, string(beBody))
		}

		c.String(http.StatusOK, strings.Join(bodies, " "))
	})
	h = router.Handler()
*/
