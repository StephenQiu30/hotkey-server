package domain

import (
	"errors"
	stdhttp "net/http"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

var (
	// ErrRefreshReplay means a refresh token had already been consumed. The
	// repository revokes its complete session family before returning it.
	ErrRefreshReplay = errors.New("refresh token replay detected")
	// ErrRefreshInvalid covers missing, expired, revoked, disabled, and
	// soft-deleted refresh-session state without disclosing which condition.
	ErrRefreshInvalid = errors.New("refresh token is invalid")
)

func InvalidCredentials() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeInvalidCredentials, stdhttp.StatusUnauthorized, "")
}

func SessionInvalid() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeSessionInvalid, stdhttp.StatusUnauthorized, "")
}

func VerificationInvalid() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeVerificationInvalid, stdhttp.StatusBadRequest, "")
}

func LastActiveAdmin() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeLastActiveAdmin, stdhttp.StatusConflict, "")
}
