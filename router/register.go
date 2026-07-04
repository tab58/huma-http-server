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
type RegisterRouteArgs[I, O, AuthInfo any] struct {
	API         huma.API
	Operation   huma.Operation
	Handler     RouteHandler[I, O, AuthInfo]
	RouteGuards []RouteGuardFunc[AuthInfo]
}

// RouteHandler is the type of handler to register for the route
type RouteHandler[I, O, AuthInfo any] func(context.Context, AuthInfo, *I) (*O, error)

// RegisterOption is the type of option for the RegisterRoute function
type RegisterOption[AuthInfo any] func(*registerOptions[AuthInfo])

// registerOptions contains the options for the RegisterRoute function
type registerOptions[AuthInfo any] struct {
	guardFns []RouteGuardFunc[AuthInfo]
}

func loadRegisterOptions[AuthInfo any](guardFns []RouteGuardFunc[AuthInfo], opts []RegisterOption[AuthInfo]) *registerOptions[AuthInfo] {
	// load the guard functions
	guards := make([]RouteGuardFunc[AuthInfo], 0)
	for _, guard := range guardFns {
		guards = append(guards, guard)
	}

	// set defaults
	o := registerOptions[AuthInfo]{
		guardFns: guards,
	}

	// apply the options
	for _, opt := range opts {
		opt(&o)
	}
	return &o
}

// RouteGuardFunc is the type of guard function to register for the route
type RouteGuardFunc[AuthInfo any] func(ctx context.Context, authInfo AuthInfo) error

func WithRouteGuard[AuthInfo any](guard RouteGuardFunc[AuthInfo]) RegisterOption[AuthInfo] {
	return func(o *registerOptions[AuthInfo]) {
		o.guardFns = append(o.guardFns, guard)
	}
}

// RegisterRoute registers a route with the given options.
func RegisterRoute[I, O any, AuthInfo map[string]string](args RegisterRouteArgs[I, O, AuthInfo], options ...RegisterOption[AuthInfo]) {
	api := args.API
	op := args.Operation
	handler := args.Handler
	routeGuards := args.RouteGuards

	// build the options
	opts := loadRegisterOptions(routeGuards, options)

	huma.Register(api, op, func(ctx context.Context, input *I) (*O, error) {
		method := op.Method
		url := op.Path
		reqID := middleware.GetRequestIDFromContext(ctx)
		authInfo := middleware.GetAuthInfoFromContext(ctx)
		authErr := middleware.GetAuthErrorFromContext(ctx)
		event := middleware.GetWideEventFromContext(ctx)

		// attach context to the wide event
		if event != nil {
			event.RequestID = reqID
			event.Method = method
			event.Path = url

			if authInfo != nil {
				event.UserID = authInfo["user_id"]
			}
			if authErr != nil {
				event.AuthError = authErr.Error()
			}
		}

		// test for route guards and run handler
		if len(opts.guardFns) > 0 {
			// guarded routes require authentication: nil auth info means the
			// request carried no valid credentials
			if authInfo == nil {
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
			statusCode := getErrorStatusCode(err)
			if event != nil {
				event.SetError(err)
				event.StatusCode = statusCode
			}
			return nil, getHumaErrorStatus(err, statusCode)
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

func getErrorStatusCode(err error) int {
	// 400 Bad Request
	if errors.Is(err, errors.ErrBadRequest) {
		return http.StatusBadRequest
	}
	// 401 Unauthorized
	if errors.Is(err, errors.ErrUnauthenticated) {
		return http.StatusUnauthorized
	}
	// 403 Forbidden
	if errors.Is(err, errors.ErrUnauthorized) {
		return http.StatusForbidden
	}
	// 404 Not Found
	if errors.Is(err, errors.ErrNotFound) {
		return http.StatusNotFound
	}
	// 500 Internal Server Error
	if errors.Is(err, errors.ErrInternalServerError) {
		return http.StatusInternalServerError
	}
	// 501 Not Implemented
	if errors.Is(err, errors.ErrNotImplemented) {
		return http.StatusNotImplemented
	}

	// default to 500 Internal Server Error
	return http.StatusInternalServerError
}

func getHumaErrorStatus(err error, statusCode int) huma.StatusError {
	switch statusCode {
	case http.StatusBadRequest: // 400
		return huma.Error400BadRequest("", err)
	case http.StatusUnauthorized: // 401
		return huma.Error401Unauthorized("", err)
	case http.StatusForbidden: // 403
		return huma.Error403Forbidden("", err)
	case http.StatusNotFound: // 404
		return huma.Error404NotFound("", err)
	case http.StatusInternalServerError: // 500
		return huma.Error500InternalServerError("", err)
	case http.StatusNotImplemented: // 501
		return huma.Error501NotImplemented("", err)
	default:
		return huma.Error500InternalServerError("", err)
	}
}
