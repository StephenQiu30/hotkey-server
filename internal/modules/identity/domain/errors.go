package domain

import (
	stdhttp "net/http"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
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
