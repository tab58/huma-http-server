package server

import "github.com/tab58/huma-http-server/router"

func RegisterRoute[I, O any, AuthInfo map[string]string](args router.RegisterRouteArgs[I, O, AuthInfo], options ...router.RegisterOption[AuthInfo]) {
	router.RegisterRoute(args, options...)
}
