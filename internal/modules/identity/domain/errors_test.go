package domain

import (
	stdhttp "net/http"
	"testing"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

func TestAuthenticationErrorsUseStableCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        *sharederrors.AppError
		code       int
		httpStatus int
	}{
		{name: "invalid credentials", err: InvalidCredentials(), code: sharederrors.CodeInvalidCredentials, httpStatus: stdhttp.StatusUnauthorized},
		{name: "session invalid", err: SessionInvalid(), code: sharederrors.CodeSessionInvalid, httpStatus: stdhttp.StatusUnauthorized},
		{name: "verification invalid", err: VerificationInvalid(), code: sharederrors.CodeVerificationInvalid, httpStatus: stdhttp.StatusBadRequest},
		{name: "last active admin", err: LastActiveAdmin(), code: sharederrors.CodeLastActiveAdmin, httpStatus: stdhttp.StatusConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.code {
				t.Errorf("Code = %d, want %d", tt.err.Code, tt.code)
			}
			if tt.err.HTTPStatus != tt.httpStatus {
				t.Errorf("HTTPStatus = %d, want %d", tt.err.HTTPStatus, tt.httpStatus)
			}
		})
	}
}
