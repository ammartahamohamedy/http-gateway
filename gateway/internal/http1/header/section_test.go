package header

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSectionValues(t *testing.T) {
	headers := Section{
		"x-test":       {"first", "\"second,third\"\tpart\xff", ""},
		"content-type": {"text/plain; charset=utf-8"},
	}

	tests := []struct {
		name    string
		headers Section
		query   string
		want    []string
	}{
		{"lowercase", headers, "x-test", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"uppercase", headers, "X-TEST", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"mixed case", headers, "x-TeSt", []string{"first", "\"second,third\"\tpart\xff", ""}},
		{"single value", headers, "content-type", []string{"text/plain; charset=utf-8"}},
		{"missing name", headers, "Host", nil},
		{"nil headers", nil, "X-Test", nil},
		{"empty headers", Section{}, "X-Test", nil},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.headers.Values(test.query))
		})
	}
}

func TestSectionValuesReturnsIndependentSlice(t *testing.T) {
	headers := Section{"x-test": {"original"}}
	values := headers.Values("x-test")
	require.Equal(t, []string{"original"}, values)
	values[0] = "changed"
	require.Equal(t, Section{"x-test": {"original"}}, headers)
}
