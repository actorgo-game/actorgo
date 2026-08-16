package httpactor

import (
	"context"
	"io"
	"maps"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	cfacade "github.com/actorgo-game/actorgo/facade"
	cproto "github.com/actorgo-game/actorgo/net/proto"
)

const (
	// HTTP uses the same body codec IDs and payload rules as AGP.
	ContentTypeJSON     = "application/json"
	ContentTypeProtobuf = "application/x-protobuf"
	// TimeoutHeader lets curl and other HTTP clients request a shorter deadline.
	TimeoutHeader = "X-ActorGo-Timeout-Ms"
	// Route is the fixed Gin route for all remotely registered Actor methods.
	Route       = "/actor/:methodID"
	routePrefix = "/actor/"
)

// Handler exposes every Methods().Register method through POST /actor/{methodID}.
type Handler struct {
	app       cfacade.IApplication
	methods   cfacade.IMethodTable
	options   Options
	requestID atomic.Uint32
}

// NewHandler creates the transport adapter over the application's method table.
func NewHandler(app cfacade.IApplication, opts ...Option) *Handler {
	if app == nil || app.Methods() == nil {
		panic("actorgo httpactor: application and method table are required")
	}
	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return &Handler{app: app, methods: app.Methods(), options: options}
}

// ServeHTTP validates the fixed route, selects JSON/PB from Content-Type and
// delegates decoding and Actor routing to the shared method table.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		h.writeError(writer, cfacade.CodecJSON, http.StatusMethodNotAllowed, "", cfacade.ErrorResult(cproto.StatusCode_STATUS_FAILED_PRECONDITION, "only POST is supported"))
		return
	}
	methodID, ok := methodIDFromPath(request.URL.Path)
	if !ok {
		h.writeError(writer, cfacade.CodecJSON, http.StatusNotFound, "", cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor method route not found"))
		return
	}
	msgType, found := h.methods.MsgType(methodID)
	if !found {
		h.writeError(writer, cfacade.CodecJSON, http.StatusNotFound, "", cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor method not found"))
		return
	}

	codec, ok := codecFromContentType(request.Header.Get("Content-Type"))
	if !ok {
		h.writeError(writer, cfacade.CodecJSON, http.StatusUnsupportedMediaType, "", cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "Content-Type must be application/json or application/x-protobuf"))
		return
	}

	requestID := h.nextRequestID()
	requestIDText := strconv.FormatUint(uint64(requestID), 10)
	session, authErr := h.authenticate(request)
	if authErr != nil {
		h.writeError(writer, codec, http.StatusUnauthorized, requestIDText, cfacade.ErrorResult(cproto.StatusCode_STATUS_UNAUTHENTICATED, authErr.Error()))
		return
	}

	request.Body = http.MaxBytesReader(writer, request.Body, h.options.MaxBodySize)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		h.writeError(writer, codec, http.StatusRequestEntityTooLarge, requestIDText, cfacade.ErrorResult(cproto.StatusCode_STATUS_RESOURCE_EXHAUSTED, "request body exceeds limit"))
		return
	}

	base, cancel := context.WithTimeout(request.Context(), h.timeout(request))
	defer cancel()
	ctx := cfacade.NewRequestContext(base)
	ctx.RequestID, ctx.Transport = requestID, cfacade.TransportHTTP
	ctx.Codec = codec
	ctx.Session, ctx.Metadata = session, h.metadata(request)

	result := h.methods.Dispatch(ctx, methodID, body, msgType)
	if msgType == cproto.MsgType_NOTIFY && result != nil && result.OK() {
		writer.WriteHeader(http.StatusAccepted)
		return
	}
	h.writeResult(writer, codec, requestIDText, ctx, result)
}

func (h *Handler) writeResult(writer http.ResponseWriter, codec int32, requestID string, ctx *cfacade.RequestContext, result *cfacade.InvokeResult) {
	if result == nil {
		result = cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "method table returned nil result")
	}
	if !result.OK() {
		h.writeError(writer, codec, httpStatus(result.Code), requestID, result)
		return
	}
	var body []byte
	if result.Payload != nil {
		if encoded, ok := result.Payload.([]byte); ok {
			body = encoded
		} else {
			encoded, err := h.app.BodyCodecs().Marshal(ctx.Codec, result.Payload)
			if err != nil {
				h.writeError(writer, codec, http.StatusInternalServerError, requestID, cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "response body encode failed"))
				return
			}
			body = encoded
		}
	}
	writer.Header().Set("Content-Type", contentType(codec))
	writer.Header().Set("X-ActorGo-Request-ID", requestID)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(body)
}

func (h *Handler) writeError(writer http.ResponseWriter, codec int32, statusCode int, requestID string, result *cfacade.InvokeResult) {
	if codec != cfacade.CodecProtobuf {
		codec = cfacade.CodecJSON
	}
	if result == nil {
		result = cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "missing invoke result")
	}
	value := &cproto.HTTPError{Code: result.Code, Message: result.Message, RequestId: requestID}
	body, err := h.app.BodyCodecs().Marshal(codec, value)
	if err != nil {
		body = []byte(`{"code":11,"message":"internal error"}`)
		codec = cfacade.CodecJSON
	}
	writer.Header().Set("Content-Type", contentType(codec))
	if requestID != "" {
		writer.Header().Set("X-ActorGo-Request-ID", requestID)
	}
	writer.WriteHeader(statusCode)
	_, _ = writer.Write(body)
}

func (h *Handler) authenticate(request *http.Request) (*cproto.Session, error) {
	if h.options.Authenticator == nil {
		return nil, nil
	}
	session, err := h.options.Authenticator(request)
	if err != nil || session == nil {
		return nil, err
	}
	// Authentication results are copied so an Actor cannot mutate state shared
	// by the authenticator or another concurrent HTTP request.
	return &cproto.Session{
		Sid:  session.Sid,
		Uid:  session.Uid,
		Ip:   session.Ip,
		Data: maps.Clone(session.Data),
	}, nil
}

func (h *Handler) timeout(request *http.Request) time.Duration {
	// A client value can shorten or extend the default but never exceed MaxTimeout.
	timeout := h.options.DefaultTimeout
	if value := request.Header.Get(TimeoutHeader); value != "" {
		if milliseconds, err := strconv.ParseUint(value, 10, 32); err == nil && milliseconds > 0 {
			timeout = time.Duration(milliseconds) * time.Millisecond
		}
	}
	if timeout > h.options.MaxTimeout {
		timeout = h.options.MaxTimeout
	}
	return timeout
}

func (h *Handler) metadata(request *http.Request) map[string][]byte {
	metadata := make(map[string][]byte)
	for _, header := range h.options.MetadataHeaders {
		if value := request.Header.Get(header); value != "" {
			metadata[strings.ToLower(header)] = []byte(value)
		}
	}
	return metadata
}

func (h *Handler) nextRequestID() uint32 {
	for {
		id := h.requestID.Add(1)
		if id != 0 {
			return id
		}
	}
}

func methodIDFromPath(path string) (uint32, bool) {
	if !strings.HasPrefix(path, routePrefix) {
		return 0, false
	}
	value := strings.TrimPrefix(path, routePrefix)
	if value == "" || strings.Contains(value, "/") {
		return 0, false
	}
	methodID, err := strconv.ParseUint(value, 10, 32)
	return uint32(methodID), err == nil && methodID > uint64(cproto.SystemMethodKick)
}

func codecFromContentType(value string) (int32, bool) {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(mediaType) {
	case ContentTypeJSON:
		return cfacade.CodecJSON, true
	case ContentTypeProtobuf:
		return cfacade.CodecProtobuf, true
	default:
		return 0, false
	}
}

func contentType(codec int32) string {
	if codec == cfacade.CodecProtobuf {
		return ContentTypeProtobuf
	}
	return ContentTypeJSON
}

func httpStatus(code int32) int {
	switch cproto.StatusCode(code) {
	case cproto.StatusCode_STATUS_OK:
		return http.StatusOK
	case cproto.StatusCode_STATUS_CANCELLED:
		return 499
	case cproto.StatusCode_STATUS_INVALID_ARGUMENT:
		return http.StatusBadRequest
	case cproto.StatusCode_STATUS_DEADLINE_EXCEEDED:
		return http.StatusGatewayTimeout
	case cproto.StatusCode_STATUS_NOT_FOUND:
		return http.StatusNotFound
	case cproto.StatusCode_STATUS_ALREADY_EXISTS, cproto.StatusCode_STATUS_ABORTED:
		return http.StatusConflict
	case cproto.StatusCode_STATUS_PERMISSION_DENIED:
		return http.StatusForbidden
	case cproto.StatusCode_STATUS_UNAUTHENTICATED:
		return http.StatusUnauthorized
	case cproto.StatusCode_STATUS_RESOURCE_EXHAUSTED:
		return http.StatusTooManyRequests
	case cproto.StatusCode_STATUS_FAILED_PRECONDITION:
		return http.StatusPreconditionFailed
	case cproto.StatusCode_STATUS_UNSUPPORTED_MEDIA:
		return http.StatusUnsupportedMediaType
	case cproto.StatusCode_STATUS_UNAVAILABLE:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
