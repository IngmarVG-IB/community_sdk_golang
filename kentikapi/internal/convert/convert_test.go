package convert_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kentik/community_sdk_golang/kentikapi/internal/convert"
)

func TestMillisecondsToDuration(t *testing.T) {
	tests := []struct {
		ms       float32
		expected time.Duration
	}{
		{
			ms:       1000,
			expected: time.Second,
		}, {
			ms:       1.1,
			expected: 1100 * time.Microsecond,
		}, {
			ms:       0.5,
			expected: 500 * time.Microsecond,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.ms), func(t *testing.T) {
			result := convert.MillisecondsF32ToDuration(tt.ms)
			assert.Equal(t, tt.expected, result)
		})
	}
}
