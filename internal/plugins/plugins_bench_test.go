package plugins

import (
	"testing"
	"time"
)

func BenchmarkPluginManager_SendAlert(b *testing.B) {
	manager := NewManager()

	// Register mock plugins
	discordPlugin := &MockPlugin{name: "discord"}
	telegramPlugin := &MockPlugin{name: "telegram"}
	emailPlugin := &MockPlugin{name: "email"}

	manager.RegisterPlugin(discordPlugin)
	manager.RegisterPlugin(telegramPlugin)
	manager.RegisterPlugin(emailPlugin)

	// Load configurations
	configs := []PluginConfig{
		{Name: "discord", Enabled: true, Config: map[string]interface{}{"webhook_url": "test"}},
		{Name: "telegram", Enabled: true, Config: map[string]interface{}{"bot_token": "test", "chat_id": "test"}},
		{Name: "email", Enabled: true, Config: map[string]interface{}{"smtp_host": "test", "smtp_port": "587", "smtp_user": "test", "smtp_pass": "test", "from": "test", "to": []string{"test"}}},
	}

	rules := []AlertRule{
		{Check: "test-check", Severity: "critical", Plugins: []string{"discord", "telegram", "email"}},
	}

	manager.LoadConfigs(configs, rules)

	// Create test check result
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

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		manager.SendAlert(checkResult)
	}
}

func BenchmarkPluginManager_LoadConfigs(b *testing.B) {
	manager := NewManager()

	// Register plugins
	discordPlugin := &MockPlugin{name: "discord"}
	telegramPlugin := &MockPlugin{name: "telegram"}

	manager.RegisterPlugin(discordPlugin)
	manager.RegisterPlugin(telegramPlugin)

	configs := []PluginConfig{
		{Name: "discord", Enabled: true, Config: map[string]interface{}{"webhook_url": "test"}},
		{Name: "telegram", Enabled: true, Config: map[string]interface{}{"bot_token": "test", "chat_id": "test"}},
	}

	rules := []AlertRule{
		{Check: "test-check", Severity: "critical", Plugins: []string{"discord", "telegram"}},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		manager.LoadConfigs(configs, rules)
	}
}

func BenchmarkPluginManager_HealthCheck(b *testing.B) {
	manager := NewManager()

	// Register plugins
	discordPlugin := &MockPlugin{name: "discord"}
	telegramPlugin := &MockPlugin{name: "telegram"}

	_ = manager.RegisterPlugin(discordPlugin)
	_ = manager.RegisterPlugin(telegramPlugin)

	configs := []PluginConfig{
		{Name: "discord", Enabled: true, Config: map[string]interface{}{"webhook_url": "test"}},
		{Name: "telegram", Enabled: true, Config: map[string]interface{}{"bot_token": "test", "chat_id": "test"}},
	}

	_ = manager.LoadConfigs(configs, []AlertRule{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = manager.HealthCheck()
	}
}

func BenchmarkAlertCreation(b *testing.B) {
	checkResult := &CheckResult{
		Name:      "test-check",
		Type:      "http",
		Status:    "down",
		Duration:  time.Second,
		Timestamp: time.Now(),
		Error:     "Connection failed",
		Device: DeviceConfig{
			Name:        "test-device",
			Location:    "test-location",
			Tags:        []string{"test", "benchmark"},
			Environment: "test",
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		alert := Alert{
			Title:     "Test Alert",
			Message:   "Test message",
			Severity:  "critical",
			Timestamp: time.Now(),
			Device:    checkResult.Device,
			Check:     checkResult,
			Metadata: map[string]interface{}{
				"test": true,
			},
		}
		_ = alert
	}
}
