package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	IngressHost       string
	IngressPathPrefix string
	TLSSecretName     string
	TraefikNamespace  string
	ChartRegistry     string
	ChartName         string
	ChartVersion      string
	ArgoCDNamespace   string
}

func NewConfig() *Config {
	return &Config{}
}

func (c *Config) Load() error {
	missing := []string{}
	// try to load each env var, if empty, append to missing
	if c.IngressHost = os.Getenv("INGRESS_HOST"); c.IngressHost == "" {
		missing = append(missing, "INGRESS_HOST")
	}
	if c.IngressPathPrefix = os.Getenv("INGRESS_PATH_PREFIX"); c.IngressPathPrefix == "" {
		missing = append(missing, "INGRESS_PATH_PREFIX")
	}
	if c.TLSSecretName = os.Getenv("TLS_SECRET_NAME"); c.TLSSecretName == "" {
		missing = append(missing, "TLS_SECRET_NAME")
	}
	if c.TraefikNamespace = os.Getenv("TRAEFIK_NAMESPACE"); c.TraefikNamespace == "" {
		missing = append(missing, "TRAEFIK_NAMESPACE")
	}
	if c.ChartRegistry = os.Getenv("CHART_REGISTRY"); c.ChartRegistry == "" {
		missing = append(missing, "CHART_REGISTRY")
	}
	if c.ChartName = os.Getenv("CHART_NAME"); c.ChartName == "" {
		missing = append(missing, "CHART_NAME")
	}
	if c.ChartVersion = os.Getenv("CHART_VERSION"); c.ChartVersion == "" {
		missing = append(missing, "CHART_VERSION")
	}
	if c.ArgoCDNamespace = os.Getenv("ARGOCD_NAMESPACE"); c.ArgoCDNamespace == "" {
		missing = append(missing, "ARGOCD_NAMESPACE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}
