package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type ctxKeyWideEvent struct{}

var WideEventContextKey ctxKeyWideEvent = ctxKeyWideEvent{}

type WideEventContext struct {
	ServiceName    string                       `json:"service_name"`    // set at creation time by the middleware
	ServiceVersion string                       `json:"service_version"` // set at creation time by the middleware
	Environment    string                       `json:"environment"`     // set at creation time by the middleware
	EventOrder     []string                     `json:"event_order"`     // set at creation time by the middleware
	Events         map[string]map[string]string `json:"events"`          // set at creation time by the middleware
	Timestamp      time.Time                    `json:"timestamp"`       // set at runtime before the handler is called by the middleware
	Duration       time.Duration                `json:"duration"`        // set at runtime before the handler is called by the middleware
	RequestID      string                       `json:"request_id"`      // set at runtime by the route handler
	Method         string                       `json:"method"`          // set at runtime by the route handler
	Path           string                       `json:"path"`            // set at runtime by the route handler
	UserID         string                       `json:"user_id"`         // set at runtime by the route handler
	StatusCode     int                          `json:"status_code"`     // set at runtime after the handler is called by the middleware
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
	return c.Error != nil || c.StatusCode >= 500
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

type WideEventConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	SampleRate     float64       // default is 0.05
	SlowThreshold  time.Duration // default is 2s
	// Logger         *slog.Logger
	SkipPaths []string
	SampleFn  func(event *WideEventContext) bool
}

// WideEvent is a middleware that attaches the wide event context to the request context.
func WideEvent(cfg WideEventConfig) func(ctx huma.Context, next func(huma.Context)) {
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
		ctx = huma.WithValue(ctx, WideEventContextKey, wideEventContext)

		// start the timer
		startTime := time.Now()
		wideEventContext.Timestamp = startTime

		next(ctx)

		// set the duration
		wideEventContext.Duration = time.Since(startTime)

		// log the wide event context
		if shouldSample(cfg, wideEventContext) {
			logWideEventContext(wideEventContext)
		}
	}
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

func logWideEventContext(event *WideEventContext) {
	jsonBytes, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal wide event context", "error", err)
		return
	}

	if event.HasError() {
		slog.Error(string(jsonBytes))
	} else {
		slog.Info(string(jsonBytes))
	}
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
