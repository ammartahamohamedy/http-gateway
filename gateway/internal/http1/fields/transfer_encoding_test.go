package fields

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTransferEncoding(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{"absent nil", nil, nil},
		{"absent empty slice", []string{}, nil},
		{"chunked", []string{"chunked"}, []string{"chunked"}},
		{"name is case-insensitive", []string{"ChUnKeD"}, []string{"chunked"}},
		{"coding list", []string{"gzip, chunked"}, []string{"gzip", "chunked"}},
		{"unfamiliar coding", []string{"x-custom, chunked"}, []string{"x-custom", "chunked"}},
		{"list order preserved", []string{"gzip, deflate, chunked"}, []string{"gzip", "deflate", "chunked"}},
		{"repeated field lines form one list", []string{"gzip", "chunked"}, []string{"gzip", "chunked"}},
		{"empty list members ignored", []string{", gzip,, chunked ,"}, []string{"gzip", "chunked"}},
		{"line holding only empty members", []string{",", "chunked"}, []string{"chunked"}},
		{"token parameter", []string{"x;a=b, chunked"}, []string{"x", "chunked"}},
		{"parameter with OWS and BWS", []string{"x ; a = b\t;\tc=d, chunked"}, []string{"x", "chunked"}},
		{"quoted parameter hides separators", []string{`x;a="1,2;3=4", chunked`}, []string{"x", "chunked"}},
		{"quoted parameter with escape", []string{`x;a="\",chunked", chunked`}, []string{"x", "chunked"}},
		{"empty quoted parameter", []string{`x;a="", chunked`}, []string{"x", "chunked"}},
		{"chunked inside quoted parameter is not a coding", []string{`x;a="1, chunked"`}, []string{"x"}},
		{"final coding need not be chunked", []string{"gzip"}, []string{"gzip"}},
		{"chunked before another coding", []string{"chunked, gzip"}, []string{"chunked", "gzip"}},
		{"chunked first across lines", []string{"chunked", "gzip"}, []string{"chunked", "gzip"}},
		{"deprecated gzip alias", []string{"X-Gzip, chunked"}, []string{"gzip", "chunked"}},
		{"deprecated compress alias", []string{"x-compress, chunked"}, []string{"compress", "chunked"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			codings, err := ParseTransferEncoding(test.values)
			require.NoError(t, err)
			require.Equal(t, test.want, codings)
		})
	}
}

func TestParseTransferEncodingRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"empty value", []string{""}},
		{"only commas", []string{" , , "}},
		{"chunked twice", []string{"chunked, chunked"}},
		{"chunked twice across lines", []string{"chunked", "chunked"}},
		{"chunked twice around another coding", []string{"chunked, gzip, chunked"}},
		{"chunked with parameter", []string{"chunked;a=b"}},
		{"chunked with empty parameter", []string{"chunked;"}},
		{"space inside name", []string{"chun ked"}},
		{"missing comma", []string{"gzip chunked"}},
		{"non-token separator", []string{"gzip/chunked"}},
		{"Kelvin sign folds to k", []string{"chunKed"}},
		{"long s folds to s", []string{"xſchunked, chunked"}},
		{"obs-text name", []string{"chunked\xff"}},
		{"NUL", []string{"chunked\x00"}},
		{"vertical tab", []string{"\x0bchunked"}},
		{"CR smuggled in name", []string{"chunked\r"}},
		{"quoted name", []string{`"chunked"`}},
		{"unterminated quoted parameter", []string{`x;a="1, chunked`}},
		{"parameter without equals", []string{"x;a, chunked"}},
		{"parameter without name", []string{"x;=b, chunked"}},
		{"parameter without value", []string{"x;a=, chunked"}},
		{"empty parameter", []string{"x;;a=b, chunked"}},
		{"parameter value with extra equals", []string{"x;a=b=c, chunked"}},
		{"text after quoted parameter", []string{`x;a="1"b, chunked`}},
		{"control byte in quoted parameter", []string{"x;a=\"\x01\", chunked"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			codings, err := ParseTransferEncoding(test.values)
			require.ErrorIs(t, err, ErrInvalidTransferEncoding)
			require.Nil(t, codings)
		})
	}
}

func TestParseTransferEncodingKeepsInput(t *testing.T) {
	values := []string{"GZIP", "Chunked"}
	_, err := ParseTransferEncoding(values)
	require.NoError(t, err)
	require.Equal(t, []string{"GZIP", "Chunked"}, values)
}

func TestValidateRequestTransferCodings(t *testing.T) {
	tests := []struct {
		name    string
		codings []string
		want    error
	}{
		{"absent", nil, nil},
		{"chunked", []string{"chunked"}, nil},
		{"coding list ending in chunked", []string{"gzip", "chunked"}, nil},
		{"final coding not chunked", []string{"gzip"}, ErrChunkedNotFinal},
		{"chunked before another coding", []string{"chunked", "gzip"}, ErrChunkedNotFinal},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateRequestTransferCodings(test.codings), test.want)
		})
	}
}

func TestCheckTransferCodingSupport(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   error
	}{
		{"no transfer coding", nil, nil},
		{"chunked", []string{"ChUnKeD"}, nil},
		{"gzip", []string{"GZIP, chunked"}, nil},
		{"deflate", []string{"Deflate, chunked"}, nil},
		{"stacked supported codings", []string{"gzip, deflate, chunked"}, nil},
		{"listed but unsupported", []string{"compress, chunked"}, ErrUnsupportedTransferCoding},
		{"unsupported after supported", []string{"gzip, compress, chunked"}, ErrUnsupportedTransferCoding},
		{"deprecated gzip alias", []string{"x-gzip, chunked"}, nil},
		{"deprecated compress alias", []string{"x-compress, chunked"}, ErrUnsupportedTransferCoding},
		{"withdrawn name", []string{"identity, chunked"}, ErrUnsupportedTransferCoding},
		{"reserved name", []string{"trailers, chunked"}, ErrUnsupportedTransferCoding},
		{"unlisted", []string{"x-custom, chunked"}, ErrUnknownTransferCoding},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			codings, err := ParseTransferEncoding(test.values)
			require.NoError(t, err)
			require.ErrorIs(t, CheckTransferCodingSupport(codings), test.want)
		})
	}
}
