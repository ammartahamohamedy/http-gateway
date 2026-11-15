package header

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHeadersValues(t *testing.T) {
	headers := Headers{
		"x-test":       {"first", "\"second,third\"\tpart\xff", ""},
		"content-type": {"text/plain; charset=utf-8"},
	}

	tests := []struct {
		name    string
		headers Headers
		query   string
		want    []string
	}{
		{"lowercase", headers, "x-test", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"uppercase", headers, "X-TEST", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"mixed case", headers, "x-TeSt", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"single value", headers, "content-type", []string{"text/plain; charset=utf-8"}},
		{"missing name", headers, "Host", nil},
		{"nil headers", nil, "X-Test", nil},
		{"empty headers", Headers{}, "X-Test", nil},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.headers.Values(test.query))
		})
	}
}

func TestHeadersValuesReturnsIndependentSlice(t *testing.T) {
	headers := Headers{"x-test": {"original"}}
	values := headers.Values("x-test")
	require.Equal(t, []string{"original"}, values)
	values[0] = "changed"
	require.Equal(t, Headers{"x-test": {"original"}}, headers)
}
