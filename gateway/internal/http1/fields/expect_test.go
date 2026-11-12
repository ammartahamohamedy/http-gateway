package fields

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseExpect(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{"absent nil", nil, nil},
		{"absent empty slice", []string{}, nil},
		{"empty value", []string{""}, nil},
		{"only commas", []string{" , , "}, nil},
		{"100-continue", []string{"100-continue"}, []string{"100-continue"}},
		{"case-insensitive", []string{"100-Continue"}, []string{"100-continue"}},
		{"unfamiliar expectation", []string{"x-custom"}, []string{"x-custom"}},
		{"expectation list", []string{"x-custom, 100-continue"}, []string{"x-custom", "100-continue"}},
		{"repeated field lines form one list", []string{"a", "b"}, []string{"a", "b"}},
		{"empty list members ignored", []string{",a,, b ,"}, []string{"a", "b"}},
		{"token value", []string{"X=Value"}, []string{"x=Value"}},
		{"quoted value hides separators", []string{`x="a,b;c=d", y`}, []string{`x="a,b;c=d"`, "y"}},
		{"value with parameters", []string{"x=1;a=b ; c=\"d\""}, []string{"x=1;a=b ; c=\"d\""}},
		{"value with empty parameters", []string{"x=1;;a=b;"}, []string{"x=1;;a=b;"}},
		{"100-continue with value stays distinct", []string{"100-Continue=x"}, []string{"100-continue=x"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			expectations, err := ParseExpect(test.values)
			require.NoError(t, err)
			require.Equal(t, test.want, expectations)
		})
	}
}

func TestParseExpectRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"space inside name", []string{"100 continue"}},
		{"missing comma", []string{"a b"}},
		{"non-token name", []string{"a/b"}},
		{"quoted name", []string{`"100-continue"`}},
		{"unterminated quote", []string{`x="1`}},
		{"parameter without value", []string{"100-continue;a=b"}},
		{"empty parameter without value", []string{"100-continue;"}},
		{"empty value", []string{"x="}},
		{"empty name", []string{"=x"}},
		{"BWS before equals", []string{"x =1"}},
		{"BWS after equals", []string{"x= 1"}},
		{"extra equals", []string{"x=1=2"}},
		{"text after quoted value", []string{`x="1"y`}},
		{"parameter missing equals", []string{"x=1;a"}},
		{"parameter with BWS", []string{"x=1;a = b"}},
		{"parameter with empty value", []string{"x=1;a="}},
		{"Kelvin sign", []string{"100-continueK"}},
		{"obs-text", []string{"x\xff"}},
		{"NUL", []string{"x\x00"}},
		{"invalid second line", []string{"x", "a b"}},
	}
	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			expectations, err := ParseExpect(test.values)
			require.ErrorIs(t, err, ErrInvalidExpect)
			require.Nil(t, expectations)
		})
	}
}

func TestParseExpectKeepsInput(t *testing.T) {
	values := []string{"100-CONTINUE", "X=1"}
	_, err := ParseExpect(values)
	require.NoError(t, err)
	require.Equal(t, []string{"100-CONTINUE", "X=1"}, values)
}

func TestValidateExpectContent(t *testing.T) {
	require.NoError(t, ValidateExpectContent(nil, false))
	require.NoError(t, ValidateExpectContent([]string{"x-custom"}, false))
	require.NoError(t, ValidateExpectContent([]string{"100-continue=x"}, false))
	require.NoError(t, ValidateExpectContent([]string{"100-continue"}, true))
	require.ErrorIs(t, ValidateExpectContent([]string{"x-custom", "100-continue"}, false), ErrExpectWithoutContent)
}
