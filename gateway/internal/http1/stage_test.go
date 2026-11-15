package http1

import (
	"bufio"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/fields"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

func TestReadRequestTagsFailedStage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		stage   Stage
		want    error
	}{
		{"EOF before request-line", "", StageRequestLine, io.EOF},
		{"request-line too long", "GET /" + strings.Repeat("a", maxRequestLineBytes) + " HTTP/1.1\r\n", StageRequestLine, ErrRequestLineTooLong},
		{"too many fields", "GET / HTTP/1.1\r\n" + strings.Repeat("X: v\r\n", requestHeaderLimits.Fields+1) + "\r\n", StageHeaders, header.ErrTooManyFields},
		{"headers cut short", "GET / HTTP/1.1\r\nHost: example.com\r\n", StageHeaders, io.ErrUnexpectedEOF},
		{"invalid Host", "GET / HTTP/1.1\r\nHost: a b\r\n\r\n", StageFields, fields.ErrInvalidHost},
		{"declared body too large", "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 9999999\r\n\r\n", StageBody, body.ErrBodyTooLarge},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			request, err := ReadRequest(bufio.NewReader(strings.NewReader(test.message)))
			require.Nil(t, request)
			var stageErr *StageError
			require.ErrorAs(t, err, &stageErr)
			require.Equal(t, test.stage, stageErr.Stage)
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestReadRequestStageErrorKeepsFieldError(t *testing.T) {
	_, err := ReadRequest(bufio.NewReader(strings.NewReader("GET / HTTP/1.1\r\nHost: a b\r\n\r\n")))
	var fieldErr *FieldError
	require.ErrorAs(t, err, &fieldErr)
	require.Equal(t, "Host", fieldErr.Name)
}

func TestReadRequestStageErrorMessageIsUnchanged(t *testing.T) {
	cause := errors.New("http1: cause")
	stageErr := &StageError{Stage: StageHeaders, Err: cause}
	require.EqualError(t, stageErr, "http1: cause")
	require.Same(t, cause, errors.Unwrap(stageErr))
}
