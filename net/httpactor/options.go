package httpactor

import (
	"net/http"
	"time"

	cproto "github.com/actorgo-game/actorgo/net/proto"
)

// Authenticator resolves the server-owned caller session before dispatching to
// an Actor. Returning nil keeps the request anonymous.
type Authenticator func(*http.Request) (*cproto.Session, error)

// Options controls global HTTP Actor limits and authentication.
type Options struct {
	MaxBodySize     int64
	DefaultTimeout  time.Duration
	MaxTimeout      time.Duration
	Authenticator   Authenticator
	MetadataHeaders []string
}

// Option mutates HTTP Actor options during construction.
type Option func(*Options)

func defaultOptions() Options {
	return Options{
		MaxBodySize:     4 << 20,
		DefaultTimeout:  3 * time.Second,
		MaxTimeout:      30 * time.Second,
		MetadataHeaders: []string{"traceparent", "tracestate", "x-request-id"},
	}
}

// WithMaxBodySize limits the encoded request body.
func WithMaxBodySize(size int64) Option {
	return func(o *Options) {
		if size > 0 {
			o.MaxBodySize = size
		}
	}
}

// WithTimeout sets the default request deadline and the client-requested cap.
func WithTimeout(defaultTimeout, maxTimeout time.Duration) Option {
	return func(o *Options) {
		if defaultTimeout > 0 {
			o.DefaultTimeout = defaultTimeout
		}
		if maxTimeout > 0 {
			o.MaxTimeout = maxTimeout
		}
	}
}

// WithAuthenticator installs one authenticator for all Actor HTTP methods.
func WithAuthenticator(authenticator Authenticator) Option {
	return func(o *Options) { o.Authenticator = authenticator }
}

// WithMetadataHeaders selects HTTP headers copied into RequestContext.Metadata.
func WithMetadataHeaders(headers ...string) Option {
	return func(o *Options) { o.MetadataHeaders = append([]string(nil), headers...) }
}
