package auth_test

import (
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

func TestPublicErrorMapsAuthenticationFailures(t *testing.T) {
	tests := []struct {
		err  error
		code enum.ErrorCode
	}{
		{service.AuthErrInvalidInput, enum.ErrorCodeAuthInvalidInput},
		{service.AuthErrInvalidCredentials, enum.ErrorCodeInvalidCredentials},
		{service.AuthErrEmailExists, enum.ErrorCodeEmailAlreadyRegistered},
		{service.AuthErrAccountDisabled, enum.ErrorCodeAccountDisabled},
		{service.VerificationErrNotFound, enum.ErrorCodeVerificationExpired},
		{service.VerificationErrInvalidCode, enum.ErrorCodeVerificationInvalid},
		{service.VerificationErrLocked, enum.ErrorCodeVerificationSendTooFrequent},
		{service.VerificationErrSendLimit, enum.ErrorCodeVerificationSendTooFrequent},
		{service.VerificationErrTicketNotFound, enum.ErrorCodeVerificationExpired},
		{service.VerificationErrTicketClaimed, enum.ErrorCodeVerificationInvalid},
		{service.ErrSessionNotFound, enum.ErrorCodeSessionExpired},
		{service.ErrSessionRevoked, enum.ErrorCodeSessionRevoked},
		{service.ErrSessionExpired, enum.ErrorCodeSessionExpired},
		{service.ErrTokenReused, enum.ErrorCodeTokenReused},
		{errors.New("dependency failed"), enum.ErrorCodeInternal},
	}

	for _, tt := range tests {
		coded := service.PublicError(tt.err)
		if coded.ErrorCode() != tt.code {
			t.Fatalf("error %v: got %s want %s", tt.err, coded.ErrorCode(), tt.code)
		}
		if !errors.Is(coded, tt.err) {
			t.Fatalf("public error must preserve cause %v", tt.err)
		}
	}
}
