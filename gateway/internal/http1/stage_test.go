package http1

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStageErrorMessageIsUnchanged(t *testing.T) {
	cause := errors.New("http1: cause")
	stageErr := &StageError{Stage: StageHeaders, Err: cause}
	require.EqualError(t, stageErr, "http1: cause")
	require.Same(t, cause, errors.Unwrap(stageErr))
}
