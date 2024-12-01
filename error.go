package ags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// ErrorCode represents a unique error identifier
type ErrorCode string

// Known error codes
const (
	ErrCodeInternal     ErrorCode = "INTERNAL_ERROR"
	ErrCodeValidation   ErrorCode = "VALIDATION_ERROR"
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrCodeNotFound     ErrorCode = "NOT_FOUND"
	ErrCodeBadRequest   ErrorCode = "BAD_REQUEST"
)

// ErrorDetail represents a single error detail
type ErrorDetail struct {
	Code    ErrorCode    `json:"code"`
	Message string       `json:"message"`
	Field   string       `json:"field,omitempty"`
	Context ErrorContext `json:"-"` // Only for logging, not serialized
}

// ErrorContext holds additional context for logging
type ErrorContext struct {
	Stack    []string          `json:"stack,omitempty"`
	Time     time.Time         `json:"time"`
	Trace    string            `json:"trace,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AppError represents our application's error type
type AppError struct {
	MainError    error           `json:"-"` // Original error
	Code         ErrorCode       `json:"code"`
	Message      string          `json:"message"`
	Details      []ErrorDetail   `json:"details,omitempty"`
	InternalLogs []string        `json:"-"` // For logging only
	Context      context.Context `json:"-"`
	StatusCode   int             `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	return e.Message
}

// NewError creates a new AppError
func NewError(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: getHTTPStatusForErrorCode(code),
		Details:    make([]ErrorDetail, 0),
		Context:    context.Background(),
	}
}

// WithError adds the original error
func (e *AppError) WithError(err error) *AppError {
	e.MainError = err
	return e
}

// WithContext adds context to the error
func (e *AppError) WithContext(ctx context.Context) *AppError {
	e.Context = ctx
	return e
}

// WithDetail adds an error detail
func (e *AppError) WithDetail(code ErrorCode, message string) *AppError {
	detail := ErrorDetail{
		Code:    code,
		Message: message,
		Context: ErrorContext{
			Time:     time.Now(),
			Metadata: make(map[string]string),
		},
	}

	// Capture stack trace
	var stack []string
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	stackLines := strings.Split(string(buf[:n]), "\n")
	for _, line := range stackLines {
		if strings.TrimSpace(line) != "" {
			stack = append(stack, strings.TrimSpace(line))
		}
	}
	detail.Context.Stack = stack

	e.Details = append(e.Details, detail)
	return e
}

// WithField adds a field-specific error detail
func (e *AppError) WithField(field, message string) *AppError {
	detail := ErrorDetail{
		Code:    ErrCodeValidation,
		Message: message,
		Field:   field,
		Context: ErrorContext{
			Time:     time.Now(),
			Metadata: make(map[string]string),
		},
	}
	e.Details = append(e.Details, detail)
	return e
}

// WithMetadata adds metadata to the last error detail
func (e *AppError) WithMetadata(key, value string) *AppError {
	if len(e.Details) > 0 {
		lastDetail := &e.Details[len(e.Details)-1]
		lastDetail.Context.Metadata[key] = value
	}
	return e
}

// AddInternalLog adds a log message for internal logging only
func (e *AppError) AddInternalLog(format string, args ...interface{}) *AppError {
	e.InternalLogs = append(e.InternalLogs, fmt.Sprintf(format, args...))
	return e
}

// getHTTPStatusForErrorCode maps error codes to HTTP status codes
func getHTTPStatusForErrorCode(code ErrorCode) int {
	switch code {
	case ErrCodeValidation, ErrCodeBadRequest:
		return http.StatusBadRequest
	case ErrCodeUnauthorized:
		return http.StatusUnauthorized
	case ErrCodeNotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// ErrorInfo represents the client-facing error information
type ErrorInfo struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// Update the error handling in the Handler struct
func (h *Handler) Error(w http.ResponseWriter, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		// Log the detailed error information
		h.cfg.Log.Error("request error",
			"code", appErr.Code,
			"message", appErr.Message,
			"details", appErr.Details,
			"internal_logs", appErr.InternalLogs,
			"original_error", appErr.MainError,
		)

		// Send simplified error response to client
		response := StandardResponse{
			OK:      false,
			Message: appErr.Message,
			Error: &ErrorInfo{
				Code:    appErr.Code,
				Message: appErr.Message,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(appErr.StatusCode)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			h.cfg.Log.Error("failed to encode JSON response", "error", err)
		}
		return
	}

	// Handle non-AppError errors
	defaultErr := NewError(ErrCodeInternal, "An internal error occurred")
	defaultErr.WithError(err).AddInternalLog("Unexpected error type: %T", err)
	h.Error(w, defaultErr)
}
