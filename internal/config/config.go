package config

import (
	"os"

	"github.com/reddit/monoceros/internal/admin"
	"github.com/reddit/monoceros/internal/supervisor"
	"gopkg.in/yaml.v3"
)

type MonocerosConfig struct {
	AdminConfig      admin.AdminConfig           `yaml:"admin"`
	SupervisorConfig supervisor.SupervisorConfig `yaml:"supervisor"`
}

func LoadConf(path string) (*MonocerosConfig, error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config MonocerosConfig
	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		return nil, err
	}

	if err := config.AdminConfig.IsValid(); err != nil {
		return nil, err
	}

	if err := config.SupervisorConfig.IsValid(); err != nil {
		return nil, err
	}

	return &config, nil
}
