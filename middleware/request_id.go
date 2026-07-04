package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
)

// Key to use when setting the request ID.
type ctxKeyRequestID int

// requestIDKey is the key that holds the unique request ID in a request context.
const requestIDKey ctxKeyRequestID = 0

// requestIDHeader is the name of the HTTP Header which contains the request id.
// Exported so that it can be changed by developers
var requestIDHeader = "X-Request-Id"

var prefix string
var reqid uint64

// A quick note on the statistics here: we're trying to calculate the chance that
// two randomly generated base62 prefixes will collide. We use the formula from
// http://en.wikipedia.org/wiki/Birthday_problem
//
// P[m, n] \approx 1 - e^{-m^2/2n}
//
// We ballpark an upper bound for $m$ by imagining (for whatever reason) a server
// that restarts every second over 10 years, for $m = 86400 * 365 * 10 = 315360000$
//
// For a $k$ character base-62 identifier, we have $n(k) = 62^k$
//
// Plugging this in, we find $P[m, n(10)] \approx 5.75%$, which is good enough for
// our purposes, and is surely more than anyone would ever need in practice -- a
// process that is rebooted a handful of times a day for a hundred years has less
// than a millionth of a percent chance of generating two colliding IDs.

func init() {
	hostname, err := os.Hostname()
	if hostname == "" || err != nil {
		hostname = "localhost"
	}
	var buf [12]byte
	var b64 string
	for len(b64) < 10 {
		// crypto/rand.Read never returns an error (Go ≥1.24); fail fast at
		// init if that contract is ever broken — a request-ID prefix from
		// uninitialized bytes would collide across processes
		if _, err := rand.Read(buf[:]); err != nil {
			panic(fmt.Sprintf("request ID init: crypto/rand failed: %v", err))
		}
		b64 = base64.StdEncoding.EncodeToString(buf[:])
		b64 = strings.NewReplacer("+", "", "/", "").Replace(b64)
	}

	prefix = fmt.Sprintf("%s/%s", hostname, b64[0:10])
}

// RequestID is a middleware that injects a request ID into the context of each
// request and echoes it back in the response header, so clients can correlate
// responses with server logs. A request ID is a string of the form
// "host.example.com/random-0001", where "random" is a base62 random string
// that uniquely identifies this go process, and where the last number is an
// atomically incremented request counter. A caller-supplied X-Request-Id is
// preserved as-is.
func RequestID() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		requestID := ctx.Header(requestIDHeader)
		if requestID == "" {
			myid := atomic.AddUint64(&reqid, 1)
			requestID = fmt.Sprintf("%s-%06d", prefix, myid)
		}
		ctx.SetHeader(requestIDHeader, requestID)
		ctx = huma.WithValue(ctx, requestIDKey, requestID)
		next(ctx)
	}
}

// GetReqID returns a request ID from the given context if one is present.
// Returns the empty string if a request ID cannot be found.
func GetRequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value(requestIDKey).(string); ok {
		return reqID
	}
	return ""
}
