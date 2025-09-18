package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestSetGlobalDisableMode(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		expected float64
	}{
		{
			name:     "global disable mode enabled",
			enabled:  true,
			expected: 1,
		},
		{
			name:     "global disable mode disabled",
			enabled:  false,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetGlobalDisableMode(tt.enabled)
			actual := testutil.ToFloat64(GlobalDisableMode)
			if actual != tt.expected {
				t.Errorf("SetGlobalDisableMode(%v) = %v, want %v", tt.enabled, actual, tt.expected)
			}
		})
	}
}
