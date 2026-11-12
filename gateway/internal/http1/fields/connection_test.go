package fields

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseConnection(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{"absent nil", nil, nil},
		{"absent empty slice", []string{}, nil},
		{"empty value", []string{""}, nil},
		{"only commas", []string{" , , "}, nil},
		{"close", []string{"close"}, []string{"close"}},
		{"case-insensitive", []string{"Keep-Alive"}, []string{"keep-alive"}},
		{"option list", []string{"close, TE"}, []string{"close", "te"}},
		{"unfamiliar option", []string{"x-hop"}, []string{"x-hop"}},
		{"repeated field lines form one list", []string{"close", "x-hop"}, []string{"close", "x-hop"}},
		{"empty list members ignored", []string{",close,, x-hop ,"}, []string{"close", "x-hop"}},
		{"duplicates preserved", []string{"close, CLOSE"}, []string{"close", "close"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			options, err := ParseConnection(test.values)
			require.NoError(t, err)
			require.Equal(t, test.want, options)
		})
	}
}

func TestParseConnectionRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"space inside option", []string{"keep alive"}},
		{"missing comma", []string{"close x-hop"}},
		{"parameter", []string{"close;a=b"}},
		{"non-token separator", []string{"close/x-hop"}},
		{"quoted option", []string{`"close"`}},
		{"quoted list", []string{`"close, x-hop"`}},
		{"unterminated quote", []string{`"close`}},
		{"Kelvin sign", []string{"Keep-alive"}},
		{"obs-text", []string{"close\xff"}},
		{"NUL", []string{"close\x00"}},
		{"CR", []string{"close\r"}},
		{"invalid second line", []string{"close", "a b"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			options, err := ParseConnection(test.values)
			require.ErrorIs(t, err, ErrInvalidConnection)
			require.Nil(t, options)
		})
	}
}

func TestParseConnectionKeepsInput(t *testing.T) {
	values := []string{"Close", "TE"}
	_, err := ParseConnection(values)
	require.NoError(t, err)
	require.Equal(t, []string{"Close", "TE"}, values)
}

func TestValidateConnectionOptions(t *testing.T) {
	for options := range slices.Values([][]string{nil, {"close"}, {"keep-alive", "te", "upgrade", "x-host", "content-type"}}) {
		require.NoError(t, ValidateConnectionOptions(options))
	}
	for options := range slices.Values([][]string{{"host"}, {"content-length"}, {"transfer-encoding"}, {"close", "host"}}) {
		require.ErrorIs(t, ValidateConnectionOptions(options), ErrProtectedConnectionOption)
	}
}
