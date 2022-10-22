package internal

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
)

type FrontendConfig struct {
	ListenAddr string
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
	s.log.Info("Frontend start")
	var h http.Handler

	// CHI

	r := chi.NewRouter()
	r.Use(chi_middleware.RequestLogger(&logger.ChiLogr{Logger: s.log}))
	r.Use(chi_middleware.Recoverer)

	r.Get("/proxy", func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("empty response")); err != nil {
				s.log.Error(err, "unable to send response")
			}

			return
		}
		defer r.Body.Close() //nolint:errcheck // not important
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err = w.Write([]byte(err.Error())); err != nil {
				s.log.Error(err, "unable to read request")
			}

			return
		}
		bodies := []string{}
		for _, beURL := range strings.Split(string(body), " ") {
			var req *http.Request
			req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, beURL, http.NoBody)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				if _, err = w.Write([]byte(err.Error())); err != nil {
					s.log.Error(err, "unable to rend request")
				}

				return
			}
			httpClient := &http.Client{}
			resp, err := httpClient.Do(req)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				if _, err = w.Write([]byte(err.Error())); err != nil {
					s.log.Error(err, "unable to send request")
				}

				return
			}
			if resp.Body == nil {
				w.WriteHeader(http.StatusInternalServerError)
				if _, err = w.Write([]byte("empty response")); err != nil {
					s.log.Error(err, "unable to get response")
				}

				return
			}
			beBody, err := io.ReadAll(resp.Body)
			resp.Body.Close() //nolint:errcheck,gosec // not important
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				if _, err = w.Write([]byte(err.Error())); err != nil {
					s.log.Error(err, "unable to read response")
				}

				return
			}
			bodies = append(bodies, string(beBody))
		}

		if _, err = w.Write([]byte(strings.Join(bodies, " "))); err != nil {
			s.log.Error(err, "unable to write response")
		}
	})
	h = r

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

	s.serverRunner(h, s.shutdown, s.config.ListenAddr, s.log)
	s.log.Info("Frontend exit")

	return nil
}
