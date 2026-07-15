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

func TestRegisterCodeRejectsDuplicate(t *testing.T) {
	definition := CodeDefinition{Code: 19999, HTTPStatus: stdhttp.StatusBadRequest, Message: "test code"}
	if err := RegisterCode(definition); err != nil {
		t.Fatalf("RegisterCode() error = %v", err)
	}
	if err := RegisterCode(definition); err == nil {
		t.Fatal("RegisterCode() error = nil, want duplicate rejection")
	}
}
