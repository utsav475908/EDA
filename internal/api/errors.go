// Package api holds HTTP plumbing shared by every service: JSON encoding,
// gRPC-compatible error bodies, and middleware.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/lakehouse-platform/lp/internal/store"
)

// Code mirrors google.rpc.Code so REST errors match grpc-gateway output.
type Code int

const (
	CodeInvalidArgument    Code = 3
	CodeNotFound           Code = 5
	CodeAlreadyExists      Code = 6
	CodePermissionDenied   Code = 7
	CodeFailedPrecondition Code = 9
	CodeInternal           Code = 13
	CodeUnauthenticated    Code = 16
)

var httpStatus = map[Code]int{
	CodeInvalidArgument:    http.StatusBadRequest,
	CodeNotFound:           http.StatusNotFound,
	CodeAlreadyExists:      http.StatusConflict,
	CodePermissionDenied:   http.StatusForbidden,
	CodeFailedPrecondition: http.StatusBadRequest,
	CodeInternal:           http.StatusInternalServerError,
	CodeUnauthenticated:    http.StatusUnauthorized,
}

type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func Errorf(code Code, msg string) *Error { return &Error{Code: code, Message: msg} }

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError maps service and store errors onto status codes. Unknown errors
// are logged and reported as Internal without leaking details.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	switch {
	case errors.As(err, &e):
	case errors.Is(err, store.ErrNotFound):
		e = Errorf(CodeNotFound, "not found")
	case errors.Is(err, store.ErrAlreadyExists):
		e = Errorf(CodeAlreadyExists, "already exists")
	default:
		slog.ErrorContext(r.Context(), "internal error", "err", err, "path", r.URL.Path)
		e = Errorf(CodeInternal, "internal error")
	}
	WriteJSON(w, httpStatus[e.Code], e)
}

// Decode reads a JSON body, rejecting unknown fields and bodies over 1 MiB.
func Decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return Errorf(CodeInvalidArgument, "invalid request body: "+err.Error())
	}
	return nil
}
