package admin

import (
	"testing"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name           string
		admin          AdminConfig
		shouldValidate bool
	}{
		{
			name: "Should Validate with valid Port",
			admin: AdminConfig{
				Port: 42,
			},
			shouldValidate: true,
		},
		{
			name: "Should not Validate with negative Port",
			admin: AdminConfig{
				Port: -42,
			},
			shouldValidate: false,
		},
	}

	for _, tt := range tests {
		err := tt.admin.IsValid()
		if got, want := err == nil, tt.shouldValidate; got != want {
			t.Errorf("IsValid() case %s = %v, want %v error: %v", tt.name, got, want, err)
		}
	}
}
