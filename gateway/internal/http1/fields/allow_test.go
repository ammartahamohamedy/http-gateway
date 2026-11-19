package fields

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAllow(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"absent", nil},
		{"empty list allows no methods", []string{""}},
		{"one method", []string{"GET"}},
		{"method list", []string{"GET, HEAD, POST"}},
		{"case preserved and unchecked", []string{"get, Custom-Method"}},
		{"repeated field lines form one list", []string{"GET", "POST"}},
		{"empty list members", []string{",GET,, HEAD ,"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateAllow(test.values))
		})
	}
}

func TestValidateAllowRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"missing comma", []string{"GET POST"}},
		{"separator", []string{"GET/POST"}},
		{"quoted method", []string{`"GET"`}},
		{"parameter", []string{"GET;q=1"}},
		{"invalid second line", []string{"GET", "P OST"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateAllow(test.values), ErrInvalidAllow)
		})
	}
}
