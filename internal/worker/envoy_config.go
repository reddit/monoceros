package worker

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"text/template"
)

//go:embed proxy/clusters_template.yaml.gotmpl
var envoyClusterConfigTemplate string

// When limiters hit there is a race condition between envoy ejecting
// the worker based on the failing health checks.
// We avoid retrying everything but we want to retry connect timeouts.
//
//go:embed proxy/listeners_template.yaml.gotmpl
var envoyListenersConfigTemplate string

//go:embed proxy/bootstrap_template.yaml.gotmpl
var envoyStartingConfigTemplate string

const envoyBootstrapConfig = "envoy_starting_config.yaml"
const envoyClusterConfig = "clusters.yaml"
const envoyListenersConfig = "listeners.yaml"

type EnvoyListenerOpt struct {
	Port uint64
}

type EnvoyClusterOpt struct {
	PortRange            []uint64
	LoadBalanceAlgorithm string
	HealthCheckPath      string
}

type EnvoyBootstrapConfiInternalgOpt struct {
	CdsPath string
	LdsPath string
	Port    uint64
}

type EnvoyBootstrapOpt struct {
	Port uint64
}

// Types of Envoy configuration
type EnvoyConfigType int

const (
	// Bootstrap config.
	Bootstrap EnvoyConfigType = iota
	// Listener config.
	Listener
	// Cluster config.
	Cluster
)

type IoWriterType int

type EnvoyConfigGenerator struct {
	wr         io.Writer
	pathPrefix string
}

func (e *EnvoyConfigGenerator) createEnvoyDir() error {
	if _, err := os.Stat(e.pathPrefix); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(e.pathPrefix, os.ModePerm)
		return err
	}

	return nil
}

// In production we use the default writer but for tests we allow the caller to inject custom writers
func (e *EnvoyConfigGenerator) getWriterOrDefault(c EnvoyConfigType) (io.Writer, error) {
	if e.wr != nil {
		return e.wr, nil
	}

	err := e.createEnvoyDir()
	if err != nil {
		return nil, err
	}

	switch c {
	case Bootstrap:
		return os.OpenFile(path.Join(e.pathPrefix, envoyBootstrapConfig), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	case Listener:
		return os.OpenFile(path.Join(e.pathPrefix, envoyListenersConfig), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	case Cluster:
		return os.OpenFile(path.Join(e.pathPrefix, envoyClusterConfig), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	default:
		return nil, fmt.Errorf("Internal error unexpected config type")
	}
}

func (e *EnvoyConfigGenerator) RenderClustersToWriter(opt EnvoyClusterOpt) error {
	t := template.Must(template.New("envoy_clusters").Parse(envoyClusterConfigTemplate))
	if t == nil {
		return fmt.Errorf("Template envoy_clusters can not be initiliased")
	}

	wr, err := e.getWriterOrDefault(Cluster)
	if err != nil {
		return err
	}

	err = t.Execute(wr, opt)
	if err != nil {
		return err
	}

	return nil
}

func (e *EnvoyConfigGenerator) RenderListenersToWriter(opt EnvoyListenerOpt) error {
	t := template.Must(template.New("envoy_listeners").Parse(envoyListenersConfigTemplate))
	if t == nil {
		return fmt.Errorf("Template envoy_listeners can not be initiliased")
	}

	wr, err := e.getWriterOrDefault(Listener)
	if err != nil {
		return err
	}

	err = t.Execute(wr, opt)
	if err != nil {
		return err
	}

	return nil
}

func (e *EnvoyConfigGenerator) RenderBootstrapConfig(opt EnvoyBootstrapOpt) error {
	t := template.Must(template.New("envoy_starting_config").Parse(envoyStartingConfigTemplate))
	if t == nil {
		return fmt.Errorf("Template envoy_starting_config can not be initiliased")
	}

	wr, err := e.getWriterOrDefault(Bootstrap)
	if err != nil {
		return err
	}

	err = t.Execute(wr, EnvoyBootstrapConfiInternalgOpt{
		CdsPath: path.Join(e.pathPrefix, envoyClusterConfig),
		LdsPath: path.Join(e.pathPrefix, envoyListenersConfig),
		Port:    opt.Port,
	})

	if err != nil {
		return err
	}

	return nil
}
