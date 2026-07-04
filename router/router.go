package router

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// Router is a huma API over a ServeMux with one server-wide AuthInfo type.
// The builder given to New converts raw JWT claims into A for every route.
type Router[A AuthInfo] struct {
	api     huma.API
	mux     *http.ServeMux
	builder AuthInfoBuilder[A]
}

func (r *Router[A]) API() huma.API {
	return r.api
}

func (r *Router[A]) Mux() *http.ServeMux {
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

func New[A AuthInfo](cfg huma.Config, builder AuthInfoBuilder[A], options ...RouterOption) *Router[A] {
	mux := http.NewServeMux()
	api := humago.New(mux, cfg)

	opts := loadRouterOptions(options)

	// attach middlewares
	for _, middleware := range opts.middlewares {
		api.UseMiddleware(middleware)
	}

	return &Router[A]{
		api:     api,
		mux:     mux,
		builder: builder,
	}
}
