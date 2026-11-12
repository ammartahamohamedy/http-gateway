package fields

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateContentEncoding(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"absent nil", nil},
		{"absent empty slice", []string{}},
		{"empty value", []string{""}},
		{"only commas", []string{" , , "}},
		{"gzip", []string{"gzip"}},
		{"mixed case", []string{"GZip"}},
		{"unfamiliar coding", []string{"x-custom"}},
		{"coding list", []string{"gzip, br"}},
		{"repeated field lines form one list", []string{"gzip", "br"}},
		{"empty list members", []string{",gzip,, br ,"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateContentEncoding(test.values))
		})
	}
}

func TestValidateContentEncodingRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"space inside coding", []string{"g zip"}},
		{"missing comma", []string{"gzip br"}},
		{"weight", []string{"gzip;q=1"}},
		{"parameter", []string{"gzip;a=b"}},
		{"non-token separator", []string{"gzip/br"}},
		{"quoted coding", []string{`"gzip"`}},
		{"unterminated quote", []string{`"gzip`}},
		{"obs-text", []string{"gzip\xff"}},
		{"NUL", []string{"gzip\x00"}},
		{"invalid second line", []string{"gzip", "b r"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateContentEncoding(test.values), ErrInvalidContentEncoding)
		})
	}
}

func TestValidateAcceptEncoding(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"absent nil", nil},
		{"absent empty slice", []string{}},
		{"empty value means identity only", []string{""}},
		{"only commas", []string{" , , "}},
		{"coding", []string{"gzip"}},
		{"mixed case", []string{"GZip"}},
		{"wildcard", []string{"*"}},
		{"identity", []string{"identity;q=0"}},
		{"browser value", []string{"gzip, deflate, br, zstd"}},
		{"weights", []string{"br;q=1.0, gzip;q=0.8, *;q=0.1"}},
		{"uppercase Q", []string{"gzip;Q=0.5"}},
		{"OWS around semicolon", []string{"gzip ; q=0.5"}},
		{"repeated field lines form one list", []string{"gzip", "br;q=0"}},
		{"empty list members", []string{",gzip,, br ,"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateAcceptEncoding(test.values))
		})
	}
}

func TestValidateAcceptEncodingRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"space inside coding", []string{"g zip"}},
		{"missing comma", []string{"gzip br"}},
		{"missing coding", []string{";q=0.5"}},
		{"empty weight", []string{"gzip;"}},
		{"q without equals", []string{"gzip;q"}},
		{"q with BWS", []string{"gzip;q = 0.5"}},
		{"q above one", []string{"gzip;q=1.1"}},
		{"uppercase Q above one", []string{"gzip;Q=2"}},
		{"q four decimals", []string{"gzip;q=0.1234"}},
		{"q quoted", []string{`gzip;q="0.5"`}},
		{"other parameter", []string{"gzip;a=b"}},
		{"second parameter", []string{"gzip;q=0.5;a=b"}},
		{"two weights", []string{"gzip;q=0.5;q=0.6"}},
		{"quoted coding", []string{`"gzip"`}},
		{"unterminated quote", []string{`"gzip`}},
		{"obs-text", []string{"gzip\xff"}},
		{"NUL", []string{"gzip\x00"}},
		{"invalid second line", []string{"gzip", "b r"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateAcceptEncoding(test.values), ErrInvalidAcceptEncoding)
		})
	}
}
