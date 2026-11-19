package fields

import (
	"slices"
	"testing"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
	"github.com/stretchr/testify/require"
)

func TestParseTrailer(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{"absent", nil, nil},
		{"empty value", []string{""}, nil},
		{"one name", []string{"X-Checksum"}, []string{"x-checksum"}},
		{"list", []string{"X-Checksum, Server-Timing"}, []string{"x-checksum", "server-timing"}},
		{"repeated field lines form one list", []string{"a", "b"}, []string{"a", "b"}},
		{"empty members ignored", []string{", a,, b ,"}, []string{"a", "b"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			names, err := ParseTrailer(test.values)
			require.NoError(t, err)
			require.Equal(t, test.want, names)
		})
	}
}

func TestParseTrailerRejectsInvalidValues(t *testing.T) {
	for values := range slices.Values([][]string{
		{"a b"}, {"a/b"}, {`"a"`}, {"a;b=c"}, {"a\x00"}, {"a", "b c"},
	}) {
		names, err := ParseTrailer(values)
		require.ErrorIs(t, err, ErrInvalidTrailer, "%q", values)
		require.Nil(t, names)
	}
}

func TestParseTrailerKeepsInput(t *testing.T) {
	values := []string{"X-Checksum"}
	_, err := ParseTrailer(values)
	require.NoError(t, err)
	require.Equal(t, []string{"X-Checksum"}, values)
}

func TestValidateTrailerNames(t *testing.T) {
	require.NoError(t, ValidateTrailerNames(nil))
	require.NoError(t, ValidateTrailerNames([]string{"x-checksum", "server-timing"}))
	for name := range slices.Values(forbiddenTrailerFields) {
		require.ErrorIs(t, ValidateTrailerNames([]string{"x-checksum", name}), ErrForbiddenTrailerField, name)
	}
}

func TestIsAllowedTrailer(t *testing.T) {
	for name := range slices.Values([]string{"x-checksum", "Server-Timing", "grpc-status"}) {
		require.True(t, IsAllowedTrailer(name), name)
	}

	for name := range slices.Values([]string{"Content-Length", "TRANSFER-ENCODING", "host", "Trailer", "Authorization", "Content-Type"}) {
		require.False(t, IsAllowedTrailer(name), name)
	}
}

func TestAllowedTrailers(t *testing.T) {
	trailers := header.Section{"x-checksum": {"abc"}, "host": {"evil.example"}, "Server-Timing": {"db;dur=5"}}

	require.Equal(t, header.Section{"x-checksum": {"abc"}, "Server-Timing": {"db;dur=5"}}, AllowedTrailers(trailers))
	require.Equal(t, header.Section{"x-checksum": {"abc"}, "host": {"evil.example"}, "Server-Timing": {"db;dur=5"}}, trailers)
}

func TestAllowedTrailersReturnsNilWhenNoneRemain(t *testing.T) {
	require.Nil(t, AllowedTrailers(nil))
	require.Nil(t, AllowedTrailers(header.Section{}))
	require.Nil(t, AllowedTrailers(header.Section{"content-length": {"5"}, "host": {"a"}}))
}
