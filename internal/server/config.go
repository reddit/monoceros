package server

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/reddit/monoceros/internal/common"
)

const proxyDefaultSignalTimeoutSec = 10

// Load-balancing algorithms suported by Envoy.
const (
	lbAlgorithmRoundRobin       = "ROUND_ROBIN"
	lbAlgorithmLeastRequest     = "LEAST_REQUEST"
	lbAlgorithmRandom           = "RANDOM"
	defaultLoadBalanceAlgorithm = lbAlgorithmRoundRobin
)

type PortArgConfig struct {
	Name          string `yaml:"name"`
	StartPort     uint64 `yaml:"start_port"`
	ValueTemplate string `yaml:"value_template"`
}

type ServerConfig struct {
	PortArg PortArgConfig `yaml:"port_arg"`
	// How much time we will wait for the proxy to handle SIGTERM
	// after that a SIGKILL is sent to the proxy
	SignalTimeout time.Duration `yaml:"signal_timeout"`
	// Path that will be used to health check the workers.
	// Existing limitations:
	// * Only http based health checking is supported
	// * Health checks are on the same port as the app. So if the worker is listening on :9090 for traffic
	// then the health check is also on :9090.
	HealthCheckPath string `yaml:"health_check_path"`
	// Load balance algorithm as supported by the proxy (envoy).
	// Defaults to `ROUND_ROBIN`.
	// envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto#enum-config-cluster-v3-cluster-lbpolicy
	// supports a bunch of algorithms he way the config is implemented we are limited to:
	// `ROUND_ROBIN`, `LEAST_REQUEST` and `RANDOM`
	LoadBalanceAlgorithm *string `yaml:"load_balancer_algorithm"`
	// Path prefix
	ProxyConfigPath string `yaml:"proxy_config_path"`
	// The admin port the proxy is going to use for the admin interface
	ProxyAdminPort int `yaml:"admin_port"`
	// The listener port the proxy is going to use for listening to connection. Defaults to 10000
	ProxyListenerPort int `yaml:"listener_port"`
}

func (s *ServerConfig) SignalTimeoutOrDefault() time.Duration {
	if s == nil || s.SignalTimeout == 0 {
		return time.Duration(proxyDefaultSignalTimeoutSec) * time.Second
	}
	return s.SignalTimeout
}

func (s *ServerConfig) LoadBalancerAlgorithmOrDefault() string {
	if s.LoadBalanceAlgorithm == nil || *s.LoadBalanceAlgorithm == "" {
		return defaultLoadBalanceAlgorithm
	}

	return *s.LoadBalanceAlgorithm
}

func (s *ServerConfig) ProxyPathPrefixOrDefault() string {
	if s.ProxyConfigPath == "" {
		return "./envoy"
	}

	return s.ProxyConfigPath
}

func (s *ServerConfig) ProxyListenerPortOrDefault() uint64 {
	if s.ProxyListenerPort <= 0 {
		return 10000
	}

	return uint64(s.ProxyListenerPort)
}

func (s *ServerConfig) IsValid() error {
	if s == nil {
		return nil
	}

	if s.PortArg.Name == "" {
		return fmt.Errorf("%w: server config name", common.ErrRequirementParameterMissing)
	}

	if s.PortArg.StartPort <= 0 {
		return fmt.Errorf("%w: server config start_port", common.ErrRequirementParameterInvalid)
	}

	if s.PortArg.ValueTemplate == "" {
		return fmt.Errorf("%w: server config value_template", common.ErrRequirementParameterMissing)
	}

	supportedLbAlgorithms := map[string]bool{
		lbAlgorithmRoundRobin:   true,
		lbAlgorithmLeastRequest: true,
		lbAlgorithmRandom:       true,
	}

	if s.LoadBalanceAlgorithm != nil {
		lb := strings.ToUpper(*s.LoadBalanceAlgorithm)
		if !supportedLbAlgorithms[lb] {
			return fmt.Errorf("%w: load_balancer_algorithm value is not supported %s", common.ErrRequirementParameterInvalid, lb)
		}
	}

	_, err := template.New("test").Parse(s.PortArg.ValueTemplate)
	if err != nil {
		return err
	}

	if s.HealthCheckPath == "" {
		return fmt.Errorf("%w: server config health_check_path", common.ErrRequirementParameterMissing)
	}

	if s.ProxyAdminPort < 0 {
		return fmt.Errorf("invalid proxy admin port")
	}

	return nil
}
