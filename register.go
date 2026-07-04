package server

import "github.com/tab58/huma-http-server/router"

// RegisterRoute registers a typed route on the server, using the server-wide
// AuthInfo type and builder given to New. The router itself stays private —
// this function is the only registration path.
func RegisterRoute[I, O any, A router.AuthInfo](s *Server[A], args router.RegisterRouteArgs[I, O, A], options ...router.RegisterOption[A]) {
	router.RegisterRoute(s.router, args, options...)
}
