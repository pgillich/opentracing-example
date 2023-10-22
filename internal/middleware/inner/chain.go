// Goroutine middlewares
package inner

import (
	"context"
)

type InternalMiddlewareFn func(ctx context.Context) (interface{}, error)

type InternalMiddleware func(next InternalMiddlewareFn) InternalMiddlewareFn

func InternalMiddlewareChain(mws ...InternalMiddleware) InternalMiddleware {
	return func(next InternalMiddlewareFn) InternalMiddlewareFn {
		fn := next
		for mw := len(mws) - 1; mw > 0; mw-- {
			fn = mws[mw](fn)
		}

		return fn
	}
}
