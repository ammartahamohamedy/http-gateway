package fields

import (
	"slices"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

func TestWithFraming(t *testing.T) {
	received := header.Section{
		"host":              {"example.com"},
		"content-length":    {"99"},
		"transfer-encoding": {"gzip, chunked"},
		"trailer":           {"x-old"},
	}

	tests := []struct {
		name         string
		mode         body.Mode
		length       int64
		trailerNames []string
		want         header.Section
	}{
		{"no body", body.None, 0, nil, header.Section{"host": {"example.com"}}},
		{"Content-Length", body.ContentLength, 5, nil,
			header.Section{"host": {"example.com"}, "content-length": {"5"}}},
		{"Content-Length zero", body.ContentLength, 0, nil,
			header.Section{"host": {"example.com"}, "content-length": {"0"}}},
		{"negative length treated as zero", body.ContentLength, -1, nil,
			header.Section{"host": {"example.com"}, "content-length": {"0"}}},
		{"chunked", body.Chunked, 0, nil,
			header.Section{"host": {"example.com"}, "transfer-encoding": {"chunked"}}},
		{"chunked with trailers", body.Chunked, 0, []string{"server-timing", "x-checksum"},
			header.Section{"host": {"example.com"}, "transfer-encoding": {"chunked"}, "trailer": {"server-timing, x-checksum"}}},
		{"trailer names ignored without chunked", body.ContentLength, 5, []string{"x-checksum"},
			header.Section{"host": {"example.com"}, "content-length": {"5"}}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, WithFraming(received, test.mode, test.length, test.trailerNames))
		})
	}
}

func TestWithFramingKeepsInput(t *testing.T) {
	received := header.Section{"host": {"example.com"}, "content-length": {"99"}}

	WithFraming(received, body.Chunked, 0, []string{"x-checksum"})
	require.Equal(t, header.Section{"host": {"example.com"}, "content-length": {"99"}}, received)
}

func TestWithFramingAcceptsNilHeaders(t *testing.T) {
	require.Equal(t, header.Section{"content-length": {"3"}}, WithFraming(nil, body.ContentLength, 3, nil))
	require.Equal(t, header.Section{}, WithFraming(nil, body.None, 0, nil))
}
