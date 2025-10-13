package wizard

import (
	"time"
)

// Template represents a configuration template
type Template struct {
	Name        string
	Description string
	Checks      []TemplateCheck
}

// TemplateCheck represents a check in a template
type TemplateCheck struct {
	Name            string
	Type            string
	URL             string
	Host            string
	Port            int
	Command         string
	ExpectedStatus  int
	Interval        time.Duration
}

// getTemplates returns available configuration templates
func getTemplates() []*Template {
	return []*Template{
		{
			Name:        "Raspberry Pi / IoT Device",
			Description: "Basic monitoring for Raspberry Pi and IoT devices",
			Checks: []TemplateCheck{
				{
					Name:     "SSH Service",
					Type:     "port",
					Host:     "localhost",
					Port:     22,
					Interval: 30 * time.Second,
				},
				{
					Name:     "System Health",
					Type:     "command",
					Command:  "uptime",
					Interval: 60 * time.Second,
				},
			},
		},
		{
			Name:        "Web Server / Application",
			Description: "Monitoring for web applications and APIs",
			Checks: []TemplateCheck{
				{
					Name:            "Homepage",
					Type:            "http",
					URL:             "http://localhost:8080",
					ExpectedStatus:  200,
					Interval:        30 * time.Second,
				},
				{
					Name:            "Health Check",
					Type:            "http",
					URL:             "http://localhost:8080/health",
					ExpectedStatus:  200,
					Interval:        30 * time.Second,
				},
				{
					Name:     "Web Server Process",
					Type:     "command",
					Command:  "pgrep -f nginx || pgrep -f apache2",
					Interval: 60 * time.Second,
				},
			},
		},
		{
			Name:        "Docker Container Host",
			Description: "Monitoring for Docker hosts and containerized applications",
			Checks: []TemplateCheck{
				{
					Name:     "Docker Daemon",
					Type:     "command",
					Command:  "docker ps",
					Interval: 30 * time.Second,
				},
				{
					Name:     "Docker Compose",
					Type:     "command",
					Command:  "docker-compose ps",
					Interval: 60 * time.Second,
				},
				{
					Name:     "Container Health",
					Type:     "command",
					Command:  "docker stats --no-stream --format 'table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}'",
					Interval: 60 * time.Second,
				},
			},
		},
		{
			Name:        "Database Server",
			Description: "Monitoring for database servers (PostgreSQL, MySQL, etc.)",
			Checks: []TemplateCheck{
				{
					Name:     "PostgreSQL",
					Type:     "port",
					Host:     "localhost",
					Port:     5432,
					Interval: 30 * time.Second,
				},
				{
					Name:     "MySQL",
					Type:     "port",
					Host:     "localhost",
					Port:     3306,
					Interval: 30 * time.Second,
				},
				{
					Name:     "Redis",
					Type:     "port",
					Host:     "localhost",
					Port:     6379,
					Interval: 30 * time.Second,
				},
				{
					Name:     "Database Connection",
					Type:     "command",
					Command:  "pg_isready -h localhost -p 5432 || mysqladmin ping -h localhost",
					Interval: 60 * time.Second,
				},
			},
		},
		{
			Name:        "Custom Configuration",
			Description: "Start with minimal configuration and add checks manually",
			Checks: []TemplateCheck{
				{
					Name:     "System Uptime",
					Type:     "command",
					Command:  "uptime",
					Interval: 60 * time.Second,
				},
			},
		},
	}
}
