package fields

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateContentType(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"absent nil", nil},
		{"absent empty slice", []string{}},
		{"media type", []string{"text/plain"}},
		{"mixed case", []string{"Text/PLAIN"}},
		{"structured suffix", []string{"application/problem+json"}},
		{"parameter", []string{"text/plain;charset=utf-8"}},
		{"OWS around semicolon", []string{"text/plain ;\tcharset=utf-8"}},
		{"several parameters", []string{"multipart/form-data; boundary=x; charset=utf-8"}},
		{"quoted parameter", []string{`multipart/form-data; boundary="a,b;c=d"`}},
		{"empty parameters", []string{"text/plain;;charset=utf-8;"}},
		{"parameter named q", []string{"text/plain;q=anything"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateContentType(test.values))
		})
	}
}

func TestValidateContentTypeRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"empty value", []string{""}},
		{"repeated field lines", []string{"text/plain", "text/plain"}},
		{"comma list", []string{"text/plain, text/html"}},
		{"missing slash", []string{"text"}},
		{"missing type", []string{"/plain"}},
		{"missing subtype", []string{"text/"}},
		{"second slash", []string{"text/plain/x"}},
		{"space around slash", []string{"text / plain"}},
		{"parameter without equals", []string{"text/plain;charset"}},
		{"parameter with BWS", []string{"text/plain;charset = utf-8"}},
		{"parameter without value", []string{"text/plain;charset="}},
		{"unterminated quote", []string{`text/plain;a="b`}},
		{"text after quoted value", []string{`text/plain;a="b"c`}},
		{"obs-text", []string{"text/pl\xffin"}},
		{"NUL", []string{"text/plain\x00"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateContentType(test.values), ErrInvalidContentType)
		})
	}
}

func TestValidateAccept(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"absent nil", nil},
		{"absent empty slice", []string{}},
		{"empty value", []string{""}},
		{"only commas", []string{" , , "}},
		{"any", []string{"*/*"}},
		{"any subtype", []string{"text/*"}},
		{"exact", []string{"text/html"}},
		{"mixed case", []string{"Text/HTML"}},
		{"browser value", []string{"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"}},
		{"repeated field lines form one list", []string{"text/html", "*/*;q=0.1"}},
		{"empty list members", []string{",text/html,,*/*,"}},
		{"parameter", []string{"text/html;level=1"}},
		{"parameter before weight", []string{"text/html;level=1;q=0.5"}},
		{"quoted parameter hides separators", []string{`text/html;a="b,c;q=9", */*`}},
		{"uppercase Q", []string{"text/html;Q=0.5"}},
		{"OWS around semicolon", []string{"text/html ; q=0.5"}},
		{"empty parameter after weight", []string{"text/html;q=0.5;"}},
		{"q zero", []string{"*/*;q=0"}},
		{"q zero point", []string{"*/*;q=0."}},
		{"q three decimals", []string{"*/*;q=0.123"}},
		{"q one", []string{"*/*;q=1"}},
		{"q one point zeros", []string{"*/*;q=1.000"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateAccept(test.values))
		})
	}
}

func TestValidateAcceptRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"missing slash", []string{"text"}},
		{"bare wildcard", []string{"*"}},
		{"wildcard type with subtype", []string{"*/html"}},
		{"missing subtype", []string{"text/"}},
		{"space around slash", []string{"text / html"}},
		{"missing comma", []string{"text/html text/plain"}},
		{"q above one", []string{"*/*;q=1.1"}},
		{"q two", []string{"*/*;q=2"}},
		{"uppercase Q above one", []string{"*/*;Q=2"}},
		{"q four decimals", []string{"*/*;q=0.1234"}},
		{"q no leading digit", []string{"*/*;q=.5"}},
		{"q negative", []string{"*/*;q=-0.5"}},
		{"q empty", []string{"*/*;q="}},
		{"q word", []string{"*/*;q=high"}},
		{"q quoted", []string{`*/*;q="0.5"`}},
		{"q without equals", []string{"*/*;q"}},
		{"q with BWS", []string{"*/*;q = 0.5"}},
		{"parameter after weight", []string{"text/html;q=0.5;level=1"}},
		{"two weights", []string{"text/html;q=0.5;q=0.6"}},
		{"parameter without equals", []string{"text/html;level"}},
		{"unterminated quote", []string{`text/html;a="b, */*`}},
		{"obs-text", []string{"text/ht\xffml"}},
		{"NUL", []string{"text/html\x00"}},
		{"invalid second line", []string{"*/*", "text"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateAccept(test.values), ErrInvalidAccept)
		})
	}
}
