package http1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestLineTargetForm(t *testing.T) {
	tests := []struct {
		rawLine string
		want    TargetForm
	}{
		{"GET / HTTP/1.1", OriginForm},
		{"GET /a?b HTTP/1.1", OriginForm},
		{"GET //a HTTP/1.1", OriginForm},
		{"GET http://example.com/ HTTP/1.1", AbsoluteForm},
		{"GET urn:example:item HTTP/1.1", AbsoluteForm},
		{"GET example.com:80 HTTP/1.1", AbsoluteForm},
		{"CONNECT example.com:443 HTTP/1.1", AuthorityForm},
		{"OPTIONS * HTTP/1.1", AsteriskForm},
		{"OPTIONS /path HTTP/1.1", OriginForm},
	}
	for _, test := range tests {
		t.Run(test.rawLine, func(t *testing.T) {
			line, err := parseRequestLine(test.rawLine)
			require.NoError(t, err)
			require.Equal(t, test.want, line.TargetForm())
		})
	}
}
