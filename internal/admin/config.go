package admin

import "fmt"

type AdminConfig struct {
	Port int `yaml:"port"`
}

func (a *AdminConfig) IsValid() error {
	if a.Port < 0 {
		return fmt.Errorf("Invalid admin port")
	}
	return nil
}
