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

func TestRegisterCodeRejectsDuplicate(t *testing.T) {
	definition := CodeDefinition{Code: 19999, HTTPStatus: stdhttp.StatusBadRequest, Message: "test code"}
	if err := RegisterCode(definition); err != nil {
		t.Fatalf("RegisterCode() error = %v", err)
	}
	if err := RegisterCode(definition); err == nil {
		t.Fatal("RegisterCode() error = nil, want duplicate rejection")
	}
}
