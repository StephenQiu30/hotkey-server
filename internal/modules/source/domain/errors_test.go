package domain

import (
	stdhttp "net/http"
	"testing"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

func TestSourceDomainErrorsUseRegisteredStableCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err    *sharederrors.AppError
		code   int
		status int
	}{
		{InvalidSourceConfiguration(), sharederrors.CodeInvalidSourceConfiguration, stdhttp.StatusBadRequest},
		{SourceConnectionRequired(), sharederrors.CodeSourceConnectionRequired, stdhttp.StatusConflict},
		{UnsupportedSourceType(), sharederrors.CodeUnsupportedSourceType, stdhttp.StatusBadRequest},
		{SourceConnectionUnavailable(), sharederrors.CodeSourceConnectionUnavailable, stdhttp.StatusConflict},
	}
	for _, test := range tests {
		if test.err.Code != test.code || test.err.HTTPStatus != test.status {
			t.Errorf("source error = %#v, want code %d status %d", test.err, test.code, test.status)
		}
	}
}
