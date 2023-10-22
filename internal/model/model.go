package model

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"
)

type contextKey string

const (
	CtxKeyCmd          = contextKey("command")
	CtxKeyServerRunner = contextKey("ServerRunner")
)

type NewService func(ctx context.Context, config interface{}) Service

type Service interface {
	Run(args []string) error
}

type ServerRunner func(h http.Handler, shutdown <-chan struct{}, addr string, l logr.Logger)
