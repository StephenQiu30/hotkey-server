package domain

import (
	stdhttp "net/http"
	"testing"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

func TestMonitorDomainErrorsUseRegisteredStableCodes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		err    *sharederrors.AppError
		code   int
		status int
	}{
		{InvalidMonitorState(), sharederrors.CodeInvalidMonitorState, stdhttp.StatusConflict},
		{MonitorVersionConflict(), sharederrors.CodeMonitorVersionConflict, stdhttp.StatusConflict},
		{InvalidMonitorConfiguration(), sharederrors.CodeInvalidMonitorConfiguration, stdhttp.StatusBadRequest},
		{MonitorDraftUnavailable(), sharederrors.CodeMonitorDraftUnavailable, stdhttp.StatusConflict},
		{MonitorNameConflict(), sharederrors.CodeMonitorNameConflict, stdhttp.StatusConflict},
	} {
		if test.err.Code != test.code || test.err.HTTPStatus != test.status {
			t.Errorf("monitor error = %#v, want stable code %d", test.err, test.code)
		}
	}
}
