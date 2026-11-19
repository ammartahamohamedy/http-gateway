package fields

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVary(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{"absent", nil, nil},
		{"empty value", []string{""}, nil},
		{"one field name", []string{"Origin"}, []string{"origin"}},
		{"field name list", []string{"Accept-Encoding, Origin"}, []string{"accept-encoding", "origin"}},
		{"asterisk", []string{"*"}, []string{"*"}},
		{"asterisk with names kept as received", []string{"Origin, *"}, []string{"origin", "*"}},
		{"repeated field lines form one list", []string{"Accept", "Origin"}, []string{"accept", "origin"}},
		{"empty list members", []string{",Origin,, Accept ,"}, []string{"origin", "accept"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			members, err := ParseVary(test.values)
			require.NoError(t, err)
			require.Equal(t, test.want, members)
		})
	}
}

func TestParseVaryRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"space inside name", []string{"Accept Encoding"}},
		{"separator", []string{"Origin/Accept"}},
		{"quoted name", []string{`"Origin"`}},
		{"invalid second line", []string{"Origin", "A ccept"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			members, err := ParseVary(test.values)
			require.ErrorIs(t, err, ErrInvalidVary)
			require.Nil(t, members)
		})
	}
}

func TestParseVaryKeepsInput(t *testing.T) {
	values := []string{"Origin, Accept"}

	_, err := ParseVary(values)
	require.NoError(t, err)
	require.Equal(t, []string{"Origin, Accept"}, values)
}
