package fields

import (
	"math"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMaxForwards(t *testing.T) {
	tests := []struct {
		name          string
		values        []string
		wantRemaining int64
		wantPresent   bool
	}{
		{"absent nil", nil, 0, false},
		{"absent empty slice", []string{}, 0, false},
		{"zero", []string{"0"}, 0, true},
		{"positive", []string{"10"}, 10, true},
		{"leading zeros", []string{"007"}, 7, true},
		{"maximum int64", []string{"9223372036854775807"}, math.MaxInt64, true},
		{"beyond int64 saturates", []string{"9223372036854775808"}, math.MaxInt64, true},
		{"far beyond int64 saturates", []string{"99999999999999999999999999"}, math.MaxInt64, true},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			remaining, present, err := ParseMaxForwards(test.values)
			require.NoError(t, err)
			require.Equal(t, test.wantRemaining, remaining)
			require.Equal(t, test.wantPresent, present)
		})
	}
}

func TestParseMaxForwardsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"empty", []string{""}},
		{"negative", []string{"-1"}},
		{"plus sign", []string{"+1"}},
		{"letters", []string{"ten"}},
		{"internal space", []string{"1 0"}},
		{"list", []string{"1, 2"}},
		{"repeated lines", []string{"1", "1"}},
	}

	for test := range slices.Values(tests) {
		t.Run(test.name, func(t *testing.T) {
			remaining, present, err := ParseMaxForwards(test.values)
			require.ErrorIs(t, err, ErrInvalidMaxForwards)
			require.Zero(t, remaining)
			require.True(t, present)
		})
	}
}
