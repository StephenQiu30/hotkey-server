package domain

import (
	stdhttp "net/http"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

func InvalidSourceConfiguration() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeInvalidSourceConfiguration, stdhttp.StatusBadRequest, "")
}

func SourceConnectionRequired() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeSourceConnectionRequired, stdhttp.StatusConflict, "")
}

func UnsupportedSourceType() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeUnsupportedSourceType, stdhttp.StatusBadRequest, "")
}

func SourceConnectionUnavailable() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeSourceConnectionUnavailable, stdhttp.StatusConflict, "")
}
