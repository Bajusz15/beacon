package plugins

import (
	"testing"
	"time"
)

func TestPluginManager(t *testing.T) {
	manager := NewManager()
	
	// Test registering plugins
	discordPlugin := &MockPlugin{name: "discord"}
	telegramPlugin := &MockPlugin{name: "telegram"}
	
	if err := manager.RegisterPlugin(discordPlugin); err != nil {
		t.Fatalf("Failed to register Discord plugin: %v", err)
	}
	
	if err := manager.RegisterPlugin(telegramPlugin); err != nil {
		t.Fatalf("Failed to register Telegram plugin: %v", err)
	}
	
	// Test loading configurations
	configs := []PluginConfig{
		{
			Name:    "discord",
			Enabled: true,
			Config: map[string]interface{}{
				"webhook_url": "https://discord.com/api/webhooks/test",
			},
		},
		{
			Name:    "telegram",
			Enabled: true,
			Config: map[string]interface{}{
				"bot_token": "test-token",
				"chat_id":   "test-chat",
			},
		},
	}
	
	rules := []AlertRule{
		{
			Check:    "test-check",
			Severity: "critical",
			Plugins:  []string{"discord", "telegram"},
			Cooldown: "5m",
		},
	}
	
	if err := manager.LoadConfigs(configs, rules); err != nil {
		t.Fatalf("Failed to load plugin configurations: %v", err)
	}
	
	// Test sending alert
	checkResult := &CheckResult{
		Name:      "test-check",
		Type:      "http",
		Status:    "down",
		Duration:  time.Second,
		Timestamp: time.Now(),
		Error:     "Connection failed",
		Device: DeviceConfig{
			Name: "test-device",
		},
	}
	
	if err := manager.SendAlert(checkResult); err != nil {
		t.Fatalf("Failed to send alert: %v", err)
	}
	
	// Verify plugins were called
	if !discordPlugin.sendAlertCalled {
		t.Error("Discord plugin SendAlert was not called")
	}
	
	if !telegramPlugin.sendAlertCalled {
		t.Error("Telegram plugin SendAlert was not called")
	}
	
	// Test health check
	if err := manager.HealthCheck(); err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	
	// Test close
	if err := manager.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// MockPlugin implements the Plugin interface for testing
type MockPlugin struct {
	name            string
	initCalled      bool
	sendAlertCalled bool
	healthCheckCalled bool
	closeCalled     bool
}

func (p *MockPlugin) Name() string {
	return p.name
}

func (p *MockPlugin) Init(config map[string]interface{}) error {
	p.initCalled = true
	return nil
}

func (p *MockPlugin) SendAlert(alert Alert) error {
	p.sendAlertCalled = true
	return nil
}

func (p *MockPlugin) HealthCheck() error {
	p.healthCheckCalled = true
	return nil
}

func (p *MockPlugin) Close() error {
	p.closeCalled = true
	return nil
}
