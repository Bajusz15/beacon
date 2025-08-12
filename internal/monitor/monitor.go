package monitor

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Checks []CheckConfig `yaml:"checks"`
	Report ReportConfig  `yaml:"report"`
}

type CheckConfig struct {
	Name         string        `yaml:"name"`
	Type         string        `yaml:"type"` // "http", "port", "command"
	URL          string        `yaml:"url,omitempty"`
	Host         string        `yaml:"host,omitempty"`
	Port         int           `yaml:"port,omitempty"`
	Cmd          string        `yaml:"cmd,omitempty"`
	Interval     time.Duration `yaml:"interval"`
	ExpectStatus int           `yaml:"expect_status,omitempty"`
}

type ReportConfig struct {
	SendTo           string `yaml:"send_to"`
	Token            string `yaml:"token"`
	PrometheusEnable bool   `yaml:"prometheus_metrics"`
	PrometheusPort   int    `yaml:"prometheus_port"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func prometheusHandler(w http.ResponseWriter, r *http.Request) {
	// Example: Replace with real check results
	fmt.Fprintf(w, "beacon_check_status{name=\"Homepage\",type=\"http\"} 1\n")
	fmt.Fprintf(w, "beacon_check_duration_seconds{name=\"Homepage\",type=\"http\"} 0.123\n")
}

func Run(cmd *cobra.Command, args []string) {
	cmd.Println("[Beacon] monitor command not yet implemented.")
}
