package ags

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
)

// debugResponseWriter wraps ResponseWriter to capture response for debug logging
type debugResponseWriter struct {
	*ResponseWriter
	handler *Handler
	request *http.Request
	buf     []byte
}

// Write captures the response data for debug logging
func (w *debugResponseWriter) Write(b []byte) (int, error) {
	if w.handler.isDebugEnabled() {
		w.buf = append(w.buf, b...)
	}
	return w.ResponseWriter.Write(b)
}

// WriteHeader captures status and generates response dump for debug logging
func (w *debugResponseWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)

	if w.handler.isDebugEnabled() {
		// Create a response for dumping
		resp := &http.Response{
			Status:     http.StatusText(status),
			StatusCode: status,
			Proto:      w.request.Proto,
			ProtoMajor: w.request.ProtoMajor,
			ProtoMinor: w.request.ProtoMinor,
			Header:     w.Header(),
			Body:       io.NopCloser(bytes.NewBuffer(w.buf)),
			Request:    w.request,
		}

		// Dump response
		respDump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			w.handler.Log(w.request.Context()).Error("failed to dump response", "error", err)
		} else {
			w.handler.Log(w.request.Context()).Debug("response dump", "dump", string(respDump))
		}
	}
}

// authenticateDebug middleware ensures valid authentication for debug endpoints
func (h *Handler) authenticateDebug(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("X-Debug-Key")

		if h.debug.authKey == "" {
			h.Error(w, NewError(ErrCodeUnauthorized, "Debug authentication not configured"))
			return
		}

		if authHeader == "" {
			h.Error(w, NewError(ErrCodeUnauthorized, "Debug key required"))
			return
		}

		if authHeader != h.debug.authKey {
			h.Error(w, NewError(ErrCodeUnauthorized, "Invalid debug key"))
			return
		}

		next(w, r)
	}
}
