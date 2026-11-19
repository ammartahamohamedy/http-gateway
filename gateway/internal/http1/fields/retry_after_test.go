package fields

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"absent", nil},
		{"delay seconds", []string{"120"}},
		{"zero delay", []string{"0"}},
		{"leading zeros", []string{"007"}},
		{"HTTP-date", []string{"Fri, 31 Dec 1999 23:59:59 GMT"}},
		{"obsolete HTTP-date", []string{"Fri Dec 31 23:59:59 1999"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateRetryAfter(test.values))
		})
	}
}

func TestValidateRetryAfterRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"empty", []string{""}},
		{"repeated field lines", []string{"120", "120"}},
		{"negative delay", []string{"-1"}},
		{"signed delay", []string{"+1"}},
		{"fractional delay", []string{"1.5"}},
		{"delay with unit", []string{"120s"}},
		{"list", []string{"120, 60"}},
		{"malformed date", []string{"Fri, 31 Dec 1999 23:59:59 UTC"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateRetryAfter(test.values), ErrInvalidRetryAfter)
		})
	}
}
