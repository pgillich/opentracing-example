package logger

import (
	"time"

	"github.com/go-logr/logr"
	"github.com/labstack/echo/v4"
)

func EchoLogr(log logr.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) { //nolint:nonamedreturns // copypasted
			start := time.Now()
			path := c.Request().URL.Path
			query := c.Request().URL.RawQuery
			if err = next(c); err != nil {
				c.Error(err)
			}

			end := time.Now()
			latency := end.Sub(start) / time.Millisecond
			if err != nil {
				log.Error(err, "?", "latency", latency)
			} else {
				log.Info(path,
					"status", c.Response().Status,
					"method", c.Request().Method,
					"path", path,
					"query", query,
					"ip", c.RealIP(),
					"user-agent", c.Request().UserAgent(),
					"latency", latency,
				)
			}

			return
		}
	}
}
