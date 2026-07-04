package router

import (
	"cmp"
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/errors"
	"github.com/tab58/huma-http-server/middleware"
)

// RegisterRouteArgs is the arguments for the RegisterRoute function
type RegisterRouteArgs[I, O any, A AuthInfo] struct {
	Operation   huma.Operation
	Handler     RouteHandler[I, O, A]
	RouteGuards []RouteGuardFunc[A]
}

// RouteHandler is the type of handler to register for the route
type RouteHandler[I, O any, A AuthInfo] func(context.Context, A, *I) (*O, error)

// RegisterOption is the type of option for the RegisterRoute function
type RegisterOption[A AuthInfo] func(*registerOptions[A])

// registerOptions contains the options for the RegisterRoute function
type registerOptions[A AuthInfo] struct {
	guardFns []RouteGuardFunc[A]
}

func loadRegisterOptions[A AuthInfo](guardFns []RouteGuardFunc[A], opts []RegisterOption[A]) *registerOptions[A] {
	// load the guard functions
	guards := make([]RouteGuardFunc[A], 0, len(guardFns))
	guards = append(guards, guardFns...)

	// set defaults
	o := registerOptions[A]{
		guardFns: guards,
	}

	// apply the options
	for _, opt := range opts {
		opt(&o)
	}
	return &o
}

// RouteGuardFunc is the type of guard function to register for the route
type RouteGuardFunc[A AuthInfo] func(ctx context.Context, authInfo A) error

func WithRouteGuard[A AuthInfo](guard RouteGuardFunc[A]) RegisterOption[A] {
	return func(o *registerOptions[A]) {
		o.guardFns = append(o.guardFns, guard)
	}
}

// RegisterRoute registers a route on the router. The router's AuthInfoBuilder
// converts raw claims into the server-wide AuthInfo type before guards run.
func RegisterRoute[I, O any, A AuthInfo](r *Router[A], args RegisterRouteArgs[I, O, A], options ...RegisterOption[A]) {
	api := r.api
	builder := r.builder
	op := args.Operation
	handler := args.Handler
	routeGuards := args.RouteGuards

	// build the options
	opts := loadRegisterOptions(routeGuards, options)

	huma.Register(api, op, func(ctx context.Context, input *I) (*O, error) {
		method := op.Method
		url := op.Path
		reqID := middleware.GetRequestIDFromContext(ctx)
		rawAuthInfo := middleware.GetAuthInfoFromContext(ctx)
		authErr := middleware.GetAuthErrorFromContext(ctx)
		event := middleware.GetWideEventFromContext(ctx)

		// attach context to the wide event
		if event != nil {
			event.RequestID = reqID
			event.Method = method
			event.Path = url

			if authErr != nil {
				event.AuthError = authErr.Error()
			}
		}

		// convert raw claims into the server-wide AuthInfo type
		authenticated := rawAuthInfo != nil
		var authInfo A
		if authenticated {
			built, err := builder(ctx, rawAuthInfo)
			if err != nil {
				statusCode := errors.MapErrorToStatus(err)
				if event != nil {
					event.SetError(err)
					event.StatusCode = statusCode
				}
				return nil, errors.MapErrorToHumaStatus(err)
			}
			authInfo = built
			if event != nil {
				event.UserID = authInfo.UserID()
			}
		}

		// test for route guards and run handler
		if len(opts.guardFns) > 0 {
			// guarded routes require authentication: no auth info means the
			// request carried no valid credentials
			if !authenticated {
				if event != nil {
					event.SetError(cmp.Or(authErr, errors.ErrUnauthenticated))
					event.StatusCode = http.StatusUnauthorized
				}
				if authErr != nil {
					// credentials were presented but invalid; detail is logged
					// via the wide event, never returned to the client
					return nil, huma.Error401Unauthorized("invalid credentials")
				}
				return nil, huma.Error401Unauthorized("authentication required")
			}
			for _, guard := range opts.guardFns {
				if err := guard(ctx, authInfo); err != nil {
					if event != nil {
						event.SetError(err)
						event.StatusCode = http.StatusForbidden
					}
					return nil, huma.Error403Forbidden("blocked by route guard", err)
				}
			}
		}
		response, err := handler(ctx, authInfo, input)

		// error
		if err != nil {
			if event != nil {
				event.SetError(err)
				event.StatusCode = errors.MapErrorToStatus(err)
			}
			return nil, errors.MapErrorToHumaStatus(err)
		}

		// success
		if event != nil {
			event.SetError(nil)
			event.StatusCode = getSuccessStatusCode(op)
		}
		return response, nil
	})
}

func getSuccessStatusCode(op huma.Operation) int {
	if op.DefaultStatus != 0 {
		return op.DefaultStatus
	}
	return http.StatusOK
}
