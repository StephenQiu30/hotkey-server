package errors

import (
	stdhttp "net/http"
	"testing"
)

func TestBaseCodesAreRegisteredAndStable(t *testing.T) {
	t.Parallel()

	for _, code := range []int{
		CodeInvalidRequest,
		CodeValidation,
		CodeConflict,
		CodeNotFound,
		CodeRateLimited,
		CodeUnauthenticated,
		CodeForbidden,
		CodeInternal,
		CodeUnavailable,
		CodeBadGateway,
		CodeDeadlineExceeded,
	} {
		definition, ok := Lookup(code)
		if !ok {
			t.Fatalf("code %d is not registered", code)
		}
		if definition.Code != code {
			t.Errorf("definition code = %d, want %d", definition.Code, code)
		}
	}
}

func TestIdentityCodesAreRegisteredWithStableHTTPStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code       int
		httpStatus int
		message    string
	}{
		{code: CodeInvalidCredentials, httpStatus: stdhttp.StatusUnauthorized, message: "invalid credentials"},
		{code: CodeSessionInvalid, httpStatus: stdhttp.StatusUnauthorized, message: "session invalid"},
		{code: CodeVerificationInvalid, httpStatus: stdhttp.StatusBadRequest, message: "verification invalid"},
		{code: CodeLastActiveAdmin, httpStatus: stdhttp.StatusConflict, message: "last active admin"},
	}

	for _, tt := range tests {
		definition, ok := Lookup(tt.code)
		if !ok {
			t.Fatalf("code %d is not registered", tt.code)
		}
		if definition.HTTPStatus != tt.httpStatus || definition.Message != tt.message {
			t.Errorf("definition = %#v, want status %d and message %q", definition, tt.httpStatus, tt.message)
		}
	}
}

func TestMonitorAndSourceCodesAreRegisteredWithStableHTTPStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code       int
		httpStatus int
	}{
		{CodeInvalidMonitorState, stdhttp.StatusConflict},
		{CodeMonitorVersionConflict, stdhttp.StatusConflict},
		{CodeInvalidMonitorConfiguration, stdhttp.StatusBadRequest},
		{CodeMonitorDraftUnavailable, stdhttp.StatusConflict},
		{CodeMonitorNameConflict, stdhttp.StatusConflict},
		{CodeInvalidSourceConfiguration, stdhttp.StatusBadRequest},
		{CodeSourceConnectionRequired, stdhttp.StatusConflict},
		{CodeUnsupportedSourceType, stdhttp.StatusBadRequest},
		{CodeSourceConnectionUnavailable, stdhttp.StatusConflict},
		{CodeCollectionRunNotFound, stdhttp.StatusNotFound},
		{CodeCollectionRunConflict, stdhttp.StatusConflict},
		{CodeInvalidCollectionRequest, stdhttp.StatusBadRequest},
	}
	for _, test := range tests {
		definition, ok := Lookup(test.code)
		if !ok {
			t.Errorf("code %d is not registered", test.code)
			continue
		}
		if definition.HTTPStatus != test.httpStatus {
			t.Errorf("code %d HTTP status = %d, want %d", test.code, definition.HTTPStatus, test.httpStatus)
		}
	}
}

func TestRegisterCodeRejectsDuplicate(t *testing.T) {
	definition := CodeDefinition{Code: 19999, HTTPStatus: stdhttp.StatusBadRequest, Message: "test code"}
	if err := RegisterCode(definition); err != nil {
		t.Fatalf("RegisterCode() error = %v", err)
	}
	if err := RegisterCode(definition); err == nil {
		t.Fatal("RegisterCode() error = nil, want duplicate rejection")
	}
}
