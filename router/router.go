package router

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

type Router struct {
	api huma.API
	mux *http.ServeMux
}

func (r *Router) API() huma.API {
	return r.api
}

func (r *Router) Mux() *http.ServeMux {
	return r.mux
}

type routerOptions struct {
	middlewares []func(ctx huma.Context, next func(huma.Context))
}

type RouterOption func(*routerOptions)

func loadRouterOptions(options []RouterOption) *routerOptions {
	opts := routerOptions{
		middlewares: make([]func(ctx huma.Context, next func(huma.Context)), 0),
	}
	for _, option := range options {
		option(&opts)
	}
	return &opts
}

func WithMiddleware(middleware func(ctx huma.Context, next func(huma.Context))) RouterOption {
	return func(o *routerOptions) {
		o.middlewares = append(o.middlewares, middleware)
	}
}

func New(cfg huma.Config, options ...RouterOption) *Router {
	mux := http.NewServeMux()
	api := humago.New(mux, cfg)

	opts := loadRouterOptions(options)

	// attach middlewares
	for _, middleware := range opts.middlewares {
		api.UseMiddleware(middleware)
	}

	return &Router{
		api: api,
		mux: mux,
	}
}
