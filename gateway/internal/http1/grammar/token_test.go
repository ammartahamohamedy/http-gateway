package grammar

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsToken(t *testing.T) {
	for text := range slices.Values([]string{"GET", "get", "Content-Type", "!#$%&'*+-.^_`|~09AZaz"}) {
		require.True(t, IsToken(text), text)
	}

	for text := range slices.Values([]string{"", " ", "a b", "a/b", "a:b", "a\x00", "Höst", "\xff", "a\r\n"}) {
		require.False(t, IsToken(text), "%q", text)
	}
}

func TestIsDigits(t *testing.T) {
	for text := range slices.Values([]string{"0", "42", "00042", "99999999999999999999"}) {
		require.True(t, IsDigits(text), text)
	}

	for text := range slices.Values([]string{"", " 1", "1 ", "+1", "-1", "1.0", "0x1", "٤", "1\n"}) {
		require.False(t, IsDigits(text), "%q", text)
	}
}

func TestIsHexDigits(t *testing.T) {
	for text := range slices.Values([]string{"0", "9", "a", "F", "0005", "DeadBeef", "ffffffffffffffffffff"}) {
		require.True(t, IsHexDigits(text), text)
	}

	for text := range slices.Values([]string{"", " 1", "1 ", "+1", "-1", "0x1", "g", "٤", "1\n"}) {
		require.False(t, IsHexDigits(text), "%q", text)
	}
}

func TestIsQValue(t *testing.T) {
	for text := range slices.Values([]string{"0", "0.", "0.5", "0.123", "1", "1.", "1.0", "1.000"}) {
		require.True(t, IsQValue(text), text)
	}

	for text := range slices.Values([]string{"", ".5", "0.1234", "1.1", "1.0000", "2", "01", "-0", "+1", "0,5", "0.5\n", " 1"}) {
		require.False(t, IsQValue(text), "%q", text)
	}
}

func TestIsFieldValueBytes(t *testing.T) {
	for octet := 0; octet <= 255; octet++ {
		t.Run(fmt.Sprintf("0x%02X", octet), func(t *testing.T) {
			want := !((octet < 0x20 && octet != '\t') || octet == 0x7f)
			require.Equal(t, want, IsFieldValue("a"+string([]byte{byte(octet)})+"z"))
		})
	}

	require.True(t, IsFieldValue(""))
}
