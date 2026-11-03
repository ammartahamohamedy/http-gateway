package grammar

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsToken(t *testing.T) {
	for _, s := range []string{"GET", "get", "Content-Type", "!#$%&'*+-.^_`|~09AZaz"} {
		require.True(t, IsToken(s), s)
	}
	for _, s := range []string{"", " ", "a b", "a/b", "a:b", "a\x00", "Höst", "\xff", "a\r\n"} {
		require.False(t, IsToken(s), "%q", s)
	}
}

func TestIsDigits(t *testing.T) {
	for _, s := range []string{"0", "42", "00042", "99999999999999999999"} {
		require.True(t, IsDigits(s), s)
	}
	for _, s := range []string{"", " 1", "1 ", "+1", "-1", "1.0", "0x1", "٤", "1\n"} {
		require.False(t, IsDigits(s), "%q", s)
	}
}

func TestIsFieldValueBytes(t *testing.T) {
	for b := 0; b <= 255; b++ {
		t.Run(fmt.Sprintf("0x%02X", b), func(t *testing.T) {
			want := !((b < 0x20 && b != '\t') || b == 0x7f)
			require.Equal(t, want, IsFieldValue("a"+string([]byte{byte(b)})+"z"))
		})
	}
	require.True(t, IsFieldValue(""))
}
