package internal

import (
	"context"

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
	c.log.Info("Client start")
	c.log.Info("Client exit")

	return nil
}
