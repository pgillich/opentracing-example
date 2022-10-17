package internal

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
)

type ClientConfig struct {
	Server string
}

type Client struct {
	config   ClientConfig
	log      logr.Logger
	shutdown <-chan struct{}
}

func NewClientService(ctx context.Context, cfg interface{}, log logr.Logger) model.Service {
	if config, is := cfg.(*ClientConfig); !is {
		log.Error(logger.ErrInvalidConfig, "config type")
		panic(logger.ErrInvalidConfig)
	} else {
		return &Client{
			config:   *config,
			log:      log,
			shutdown: ctx.Done(),
		}
	}
}

func (c *Client) Run(args []string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+c.config.Server+"/proxy", strings.NewReader(strings.Join(args, " ")))
	if err != nil {
		return err
	}
	httpClient := http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close() //nolint:errcheck // not needed
	}
	c.log.Info("Client resp", "body", string(body))

	return nil
}
