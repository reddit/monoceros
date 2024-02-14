package server

import (
	"testing"
)

func TestConfigValidation(t *testing.T) {
	randomString := "foo"
	validLbAlgorithm := lbAlgorithmRoundRobin
	tests := []struct {
		name           string
		s              ServerConfig
		shouldValidate bool
	}{
		{
			name: "Should Validate if valid",
			s: ServerConfig{
				PortArg: PortArgConfig{
					Name:          "foo",
					StartPort:     9000,
					ValueTemplate: "{{.Port}}",
				},
				HealthCheckPath:      "/foo",
				LoadBalanceAlgorithm: &validLbAlgorithm,
			},
			shouldValidate: true,
		},
		{
			name: "Should Validate if valid (only minimum required fields)",
			s: ServerConfig{
				PortArg: PortArgConfig{
					Name:          "foo",
					StartPort:     9000,
					ValueTemplate: "{{.Port}}",
				},
				HealthCheckPath: "/foo",
			},
			shouldValidate: true,
		},
		{
			name: "Should not Validate if name is empty",
			s: ServerConfig{
				PortArg: PortArgConfig{
					StartPort:     9000,
					ValueTemplate: "{{.Port}}",
				},
				HealthCheckPath: "/foo",
			},
			shouldValidate: false,
		},
		{
			name: "Should not Validate if start_port is invalid",
			s: ServerConfig{
				PortArg: PortArgConfig{
					Name:          "foo",
					StartPort:     0,
					ValueTemplate: "{{.Port}}",
				},
				HealthCheckPath: "/foo",
			},
			shouldValidate: false,
		},
		{
			name: "Should not Validate if value_template is empty",
			s: ServerConfig{
				PortArg: PortArgConfig{
					Name:      "foo",
					StartPort: 9000,
				},
				HealthCheckPath: "/foo",
			},
			shouldValidate: false,
		},
		{
			name: "Should not Validate if health check path is empty",
			s: ServerConfig{
				PortArg: PortArgConfig{
					Name:      "foo",
					StartPort: 9000,
				},
				HealthCheckPath: "",
			},
			shouldValidate: false,
		},
		{
			name: "Should not Validate if load balancer algorithm is not supported",
			s: ServerConfig{
				PortArg: PortArgConfig{
					Name:      "foo",
					StartPort: 9000,
				},
				HealthCheckPath:      "/foo",
				LoadBalanceAlgorithm: &randomString,
			},
			shouldValidate: false,
		},
		{
			name: "Should not Validate if load balancer algorithm is not supported",
			s: ServerConfig{
				PortArg: PortArgConfig{
					Name:      "foo",
					StartPort: 9000,
				},
				HealthCheckPath:      "/foo",
				LoadBalanceAlgorithm: &randomString,
			},
			shouldValidate: false,
		},
		{
			name: "Should not Validate if proxy admin port is <0",
			s: ServerConfig{
				PortArg: PortArgConfig{
					Name:      "foo",
					StartPort: 9000,
				},
				HealthCheckPath:      "/foo",
				LoadBalanceAlgorithm: &validLbAlgorithm,
				ProxyAdminPort:       -1,
			},
			shouldValidate: false,
		},
	}

	for _, tt := range tests {
		err := tt.s.IsValid()
		if got, want := err == nil, tt.shouldValidate; got != want {
			t.Errorf("IsValid() case %s = %v, want %v error: %v", tt.name, got, want, err)
		}
	}

}
