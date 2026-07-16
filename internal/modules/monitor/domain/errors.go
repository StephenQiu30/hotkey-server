package domain

import (
	stdhttp "net/http"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

func InvalidMonitorState() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeInvalidMonitorState, stdhttp.StatusConflict, "")
}

func MonitorVersionConflict() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeMonitorVersionConflict, stdhttp.StatusConflict, "")
}

func InvalidMonitorConfiguration() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeInvalidMonitorConfiguration, stdhttp.StatusBadRequest, "")
}

func MonitorDraftUnavailable() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeMonitorDraftUnavailable, stdhttp.StatusConflict, "")
}

func MonitorNameConflict() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeMonitorNameConflict, stdhttp.StatusConflict, "")
}
