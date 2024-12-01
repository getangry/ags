package ags_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getangry/ags"
)

func TestNewError(t *testing.T) {
	tests := []struct {
		name         string
		code         ags.ErrorCode
		message      string
		wantHTTPCode int
	}{
		{
			name:         "internal error",
			code:         ags.ErrCodeInternal,
			message:      "internal server error",
			wantHTTPCode: http.StatusInternalServerError,
		},
		{
			name:         "validation error",
			code:         ags.ErrCodeValidation,
			message:      "validation failed",
			wantHTTPCode: http.StatusBadRequest,
		},
		{
			name:         "unauthorized error",
			code:         ags.ErrCodeUnauthorized,
			message:      "unauthorized access",
			wantHTTPCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ags.NewError(tt.code, tt.message)

			if err.Code != tt.code {
				t.Errorf("NewError() code = %v, want %v", err.Code, tt.code)
			}
			if err.Message != tt.message {
				t.Errorf("NewError() message = %v, want %v", err.Message, tt.message)
			}
			if err.StatusCode != tt.wantHTTPCode {
				t.Errorf("NewError() status code = %v, want %v", err.StatusCode, tt.wantHTTPCode)
			}
			if len(err.Details) != 0 {
				t.Errorf("NewError() details should be empty, got %v", len(err.Details))
			}
		})
	}
}

func TestAppError_WithError(t *testing.T) {
	originalErr := errors.New("original error")
	appErr := ags.NewError(ags.ErrCodeInternal, "wrapped error").WithError(originalErr)

	if appErr.MainError != originalErr {
		t.Errorf("WithError() mainError = %v, want %v", appErr.MainError, originalErr)
	}
}

func TestAppError_WithContext(t *testing.T) {
	type contextKey string
	var key contextKey = "key"
	ctx := context.WithValue(context.Background(), key, "value")
	appErr := ags.NewError(ags.ErrCodeInternal, "error with context").WithContext(ctx)

	if appErr.Context != ctx {
		t.Error("WithContext() did not set the expected context")
	}
}

func TestAppError_WithDetail(t *testing.T) {
	appErr := ags.NewError(ags.ErrCodeInternal, "main error")
	appErr.WithDetail(ags.ErrCodeValidation, "detail message")

	if len(appErr.Details) != 1 {
		t.Fatalf("WithDetail() details count = %v, want 1", len(appErr.Details))
	}

	detail := appErr.Details[0]
	if detail.Code != ags.ErrCodeValidation {
		t.Errorf("WithDetail() detail code = %v, want %v", detail.Code, ags.ErrCodeValidation)
	}
	if detail.Message != "detail message" {
		t.Errorf("WithDetail() detail message = %v, want 'detail message'", detail.Message)
	}
	if detail.Context.Stack == nil {
		t.Error("WithDetail() stack trace should not be nil")
	}
}

func TestAppError_WithField(t *testing.T) {
	appErr := ags.NewError(ags.ErrCodeValidation, "validation error").
		WithField("email", "invalid email format")

	if len(appErr.Details) != 1 {
		t.Fatalf("WithField() details count = %v, want 1", len(appErr.Details))
	}

	detail := appErr.Details[0]
	if detail.Code != ags.ErrCodeValidation {
		t.Errorf("WithField() detail code = %v, want %v", detail.Code, ags.ErrCodeValidation)
	}
	if detail.Field != "email" {
		t.Errorf("WithField() field = %v, want 'email'", detail.Field)
	}
	if detail.Message != "invalid email format" {
		t.Errorf("WithField() message = %v, want 'invalid email format'", detail.Message)
	}
}

func TestAppError_WithMetadata(t *testing.T) {
	appErr := ags.NewError(ags.ErrCodeInternal, "error")
	appErr.WithDetail(ags.ErrCodeValidation, "detail").WithMetadata("key", "value")

	if len(appErr.Details) != 1 {
		t.Fatalf("WithMetadata() details count = %v, want 1", len(appErr.Details))
	}

	detail := appErr.Details[0]
	if value, exists := detail.Context.Metadata["key"]; !exists || value != "value" {
		t.Errorf("WithMetadata() metadata = %v, want map[key:value]", detail.Context.Metadata)
	}
}

func TestAppError_AddInternalLog(t *testing.T) {
	appErr := ags.NewError(ags.ErrCodeInternal, "error")
	appErr.AddInternalLog("log message %s", "test")

	if len(appErr.InternalLogs) != 1 {
		t.Fatalf("AddInternalLog() logs count = %v, want 1", len(appErr.InternalLogs))
	}

	if appErr.InternalLogs[0] != "log message test" {
		t.Errorf("AddInternalLog() log message = %v, want 'log message test'", appErr.InternalLogs[0])
	}
}

func TestHandler_Error(t *testing.T) {
	mockLog := &mockLogger{}
	handler := ags.NewHandler(&ags.ServerConfig{
		Log: mockLog,
	})

	tests := []struct {
		name          string
		err           error
		wantStatus    int
		wantErrorCode ags.ErrorCode
	}{
		{
			name:          "app error",
			err:           ags.NewError(ags.ErrCodeValidation, "validation failed"),
			wantStatus:    http.StatusBadRequest,
			wantErrorCode: ags.ErrCodeValidation,
		},
		{
			name:          "standard error",
			err:           errors.New("standard error"),
			wantStatus:    http.StatusInternalServerError,
			wantErrorCode: ags.ErrCodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.Error(w, tt.err) // Assuming you export this method for testing

			if w.Code != tt.wantStatus {
				t.Errorf("Error() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if !strings.Contains(w.Body.String(), string(tt.wantErrorCode)) {
				t.Errorf("Error() body = %v, want to contain %v", w.Body.String(), tt.wantErrorCode)
			}
		})
	}
}
