package worker

import (
	"bufio"
	"bytes"
	_ "embed"
	"testing"
)

//go:embed test_data/clusters.yaml
var wantClustersConfig string

//go:embed test_data/listeners.yaml
var wantListenerConfig string

//go:embed test_data/envoy_starting_config.yaml
var wantBootstrapConfig string

func TestClusterRender(t *testing.T) {
	e := EnvoyConfigGenerator{
		pathPrefix: "envoy",
	}

	var b bytes.Buffer
	outW := bufio.NewWriter(&b)
	e.wr = outW
	opt := EnvoyClusterOpt{
		PortRange:            []uint64{900, 901, 902, 903, 904},
		LoadBalanceAlgorithm: "ROUND_ROBIN",
		HealthCheckPath:      "/get",
	}

	if err := e.RenderClustersToWriter(opt); err != nil {
		t.Fatalf("TestClusterRender() unexpected error %v", err)
	}

	outW.Flush()
	gotClustersConfig := b.String()
	if gotClustersConfig != wantClustersConfig {
		t.Errorf("Invalid cluster config generated \n"+
			"------- Got ------ \n"+
			"%s"+
			"------- Want ------ \n"+
			"%s", gotClustersConfig, wantClustersConfig)
	}
}

func TestListenerRender(t *testing.T) {
	e := EnvoyConfigGenerator{}

	var b bytes.Buffer
	outW := bufio.NewWriter(&b)
	e.wr = outW

	opt := EnvoyListenerOpt{
		Port: 42,
	}

	if err := e.RenderListenersToWriter(opt); err != nil {
		t.Fatalf("TestClusterRender() unexpected error %v", err)
	}

	outW.Flush()
	gotListenerConfig := b.String()
	if gotListenerConfig != wantListenerConfig {
		t.Errorf("Invalid cluster config generated \n"+
			"------- Got ------ \n"+
			"%s"+
			"------- Want ------ \n"+
			"%s", gotListenerConfig, wantListenerConfig)
	}
}

func TestBootstrapRender(t *testing.T) {
	e := EnvoyConfigGenerator{
		pathPrefix: "envoy",
	}

	var b bytes.Buffer
	outW := bufio.NewWriter(&b)
	e.wr = outW

	bOpt := EnvoyBootstrapOpt{
		Port: 9901,
	}
	if err := e.RenderBootstrapConfig(bOpt); err != nil {
		t.Fatalf("TestClusterRender() unexpected error %v", err)
	}

	outW.Flush()
	gotBootstrapConfig := b.String()
	if gotBootstrapConfig != wantBootstrapConfig {
		t.Errorf("Invalid cluster config generated \n"+
			"------- Got ------ \n"+
			"%s"+
			"------- Want ------ \n"+
			"%s", gotBootstrapConfig, wantBootstrapConfig)
	}
}
