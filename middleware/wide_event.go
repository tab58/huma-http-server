package middleware

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type ctxKeyWideEvent struct{}

var WideEventContextKey ctxKeyWideEvent = ctxKeyWideEvent{}

type WideEventContext struct {
	ServiceName    string                       `json:"service_name"`         // set at creation time by the middleware
	ServiceVersion string                       `json:"service_version"`      // set at creation time by the middleware
	Environment    string                       `json:"environment"`          // set at creation time by the middleware
	EventOrder     []string                     `json:"event_order"`          // set at creation time by the middleware
	Events         map[string]map[string]string `json:"events"`               // set at creation time by the middleware
	Timestamp      time.Time                    `json:"timestamp"`            // set at runtime before the handler is called by the middleware
	Duration       time.Duration                `json:"duration"`             // set at runtime before the handler is called by the middleware
	RequestID      string                       `json:"request_id"`           // set at runtime by the route handler
	Method         string                       `json:"method"`               // set at runtime by the route handler
	Path           string                       `json:"path"`                 // set at runtime by the route handler
	UserID         string                       `json:"user_id"`              // set at runtime by the route handler
	StatusCode     int                          `json:"status_code"`          // set at runtime after the handler is called by the middleware
	AuthError      string                       `json:"auth_error,omitempty"` // set at runtime by the route handler when presented credentials failed verification
	Error          error                        `json:"-"`
	ErrorMessage   string                       `json:"error,omitempty"`
}

func newContext(cfg WideEventConfig) *WideEventContext {
	return &WideEventContext{
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Environment:    cfg.Environment,
		EventOrder:     make([]string, 0),
		Events:         make(map[string]map[string]string),
	}
}

func (c *WideEventContext) HasError() bool {
	return c.Error != nil || c.StatusCode >= 400
}

func (c *WideEventContext) SetError(err error) {
	c.Error = err
	if err != nil {
		c.ErrorMessage = err.Error()
	} else {
		c.ErrorMessage = ""
	}
}

func (c *WideEventContext) AttachEventContext(eventType string, eventData map[string]string) {
	c.EventOrder = append(c.EventOrder, eventType)
	c.Events[eventType] = eventData
}

const DEFAULT_SAMPLE_RATE = 0.05
const DEFAULT_SLOW_THRESHOLD = 2 * time.Second

type WideEventConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	SampleRate     float64       // fraction of successful requests to log; 0 disables success sampling
	SlowThreshold  time.Duration // 0 means DEFAULT_SLOW_THRESHOLD (2s)
	Logger         *slog.Logger  // nil means slog.Default()
	SkipPaths      []string
	SampleFn       func(event *WideEventContext) bool
}

// applyWideEventDefaults returns a copy of cfg with documented defaults
// applied to zero values. SampleRate deliberately has no zero-default:
// 0 must mean "never sample successes", not "use 0.05".
func applyWideEventDefaults(cfg WideEventConfig) WideEventConfig {
	if cfg.SlowThreshold == 0 {
		cfg.SlowThreshold = DEFAULT_SLOW_THRESHOLD
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}

// WideEvent is a middleware that attaches the wide event context to the request context.
func WideEvent(cfg WideEventConfig) func(ctx huma.Context, next func(huma.Context)) {
	cfg = applyWideEventDefaults(cfg)
	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	return func(ctx huma.Context, next func(huma.Context)) {
		if _, skip := skipSet[ctx.URL().Path]; skip {
			next(ctx)
			return
		}

		wideEventContext := newContext(cfg)
		// stamp request identity here so events emitted without reaching a
		// route handler (e.g. huma 422 validation failures) still carry it;
		// RegisterRoute later overwrites Path with the route template
		wideEventContext.RequestID = GetRequestIDFromContext(ctx.Context())
		wideEventContext.Method = ctx.Method()
		wideEventContext.Path = ctx.URL().Path
		ctx = huma.WithValue(ctx, WideEventContextKey, wideEventContext)

		// start the timer
		startTime := time.Now()
		wideEventContext.Timestamp = startTime

		next(ctx)

		// set the duration
		wideEventContext.Duration = time.Since(startTime)

		// requests rejected before the route wrapper runs (validation
		// failures) never stamp a status — take it from the response
		if wideEventContext.StatusCode == 0 {
			wideEventContext.StatusCode = ctx.Status()
		}

		// log the wide event context
		if shouldSample(cfg, wideEventContext) {
			logWideEventContext(cfg, wideEventContext)
		}
	}
}

// WideEventNotFound wraps the mux to log wide events for requests matching no
// registered route (404/405). Huma middlewares never run for those, so
// without this they produce no event at all. Raw Handle() routes match a mux
// pattern and are not logged here.
func WideEventNotFound(cfg WideEventConfig, mux *http.ServeMux) http.Handler {
	cfg = applyWideEventDefaults(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}

		event := newContext(cfg)
		event.Method = r.Method
		event.Path = r.URL.Path
		startTime := time.Now()
		event.Timestamp = startTime

		rec := &statusRecorder{ResponseWriter: w}
		mux.ServeHTTP(rec, r)

		event.Duration = time.Since(startTime)
		event.StatusCode = rec.status
		if shouldSample(cfg, event) {
			logWideEventContext(cfg, event)
		}
	})
}

// statusRecorder captures the response status code. Only used on unmatched
// routes, where the response is a plain error write — no Flusher/Hijacker
// passthrough needed.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// shouldSample determines if the wide event context should be sampled.
// Tail sampling is used to reduce the volume of logs.
func shouldSample(cfg WideEventConfig, event *WideEventContext) bool {
	// always keep errors
	if event.HasError() {
		return true
	}

	// always keep slow requests
	if event.Duration > cfg.SlowThreshold {
		return true
	}

	// sample using the configured function
	if cfg.SampleFn != nil && cfg.SampleFn(event) {
		return true
	}

	// sample at the configured rate
	return rand.Float64() < cfg.SampleRate
}

func logWideEventContext(cfg WideEventConfig, event *WideEventContext) {
	level := slog.LevelInfo
	if event.HasError() {
		level = slog.LevelError
	}
	// structured attr, not a pre-encoded JSON string — a JSON slog handler
	// encodes the event as a nested object instead of double-encoding it
	cfg.Logger.LogAttrs(context.Background(), level, "wide_event", slog.Any("event", event))
}

// GetAuthInfoFromContext gets the authentication info from the context, no matter how buried it is.
// This will only work if the authentication middleware has been applied.
func GetWideEventFromContext(ctx context.Context) *WideEventContext {
	if ctx == nil {
		return nil
	}
	if authInfo, ok := ctx.Value(WideEventContextKey).(*WideEventContext); ok {
		return authInfo
	}
	return nil
}
