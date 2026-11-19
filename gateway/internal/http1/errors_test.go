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

func TestFieldErrorMessageIsUnchanged(t *testing.T) {
	cause := errors.New("http1: cause")
	fieldErr := &FieldError{Name: "Content-Length", Err: cause}
	require.EqualError(t, fieldErr, "http1: cause")
	require.Same(t, cause, errors.Unwrap(fieldErr))
}

func TestStageErrorKeepsFieldError(t *testing.T) {
	cause := errors.New("http1: cause")
	var err error = &StageError{Stage: StageFields, Err: &FieldError{Name: "Vary", Err: cause}}

	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Vary", fieldErr.Name)
	require.ErrorIs(t, err, cause)
}
