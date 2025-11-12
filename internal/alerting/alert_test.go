package alerting

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleAlertManager_Comprehensive(t *testing.T) {
	t.Run("initialization", func(t *testing.T) {
		sam := NewSimpleAlertManager()

		assert.NotNil(t, sam)
		assert.NotNil(t, sam.routing)
		assert.NotNil(t, sam.activeAlerts)
		assert.NotNil(t, sam.cooldowns)
		assert.Equal(t, 0, len(sam.routing))
		assert.Equal(t, 0, len(sam.activeAlerts))
		assert.Equal(t, 0, len(sam.cooldowns))
	})

	t.Run("routing_configuration", func(t *testing.T) {
		sam := NewSimpleAlertManager()

		// Test valid routing
		validRouting := []AlertRouting{
			{
				Severity:         SeverityCritical,
				Channels:         []string{"email", "webhook"},
				Recipients:       []string{"admin@example.com", "#alerts"},
				BackupDelay:      30 * time.Minute,
				BackupRecipients: []string{"backup@example.com"},
				Enabled:          true,
			},
			{
				Severity:   SeverityWarning,
				Channels:   []string{"slack"},
				Recipients: []string{"#warnings"},
				Enabled:    true,
			},
		}

		err := sam.LoadRouting(validRouting)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(sam.routing))

		// Test loop variable fix - verify each routing has correct values
		criticalRouting := sam.routing[SeverityCritical]
		warningRouting := sam.routing[SeverityWarning]

		assert.Equal(t, SeverityCritical, criticalRouting.Severity)
		assert.Equal(t, []string{"email", "webhook"}, criticalRouting.Channels)
		assert.Equal(t, []string{"admin@example.com", "#alerts"}, criticalRouting.Recipients)

		assert.Equal(t, SeverityWarning, warningRouting.Severity)
		assert.Equal(t, []string{"slack"}, warningRouting.Channels)
		assert.Equal(t, []string{"#warnings"}, warningRouting.Recipients)

		// Test validation errors
		invalidRoutings := []struct {
			name     string
			routing  []AlertRouting
			errorMsg string
		}{
			{
				name: "empty_severity",
				routing: []AlertRouting{
					{Severity: "", Channels: []string{"email"}, Enabled: true},
				},
				errorMsg: "alert routing severity cannot be empty",
			},
			{
				name: "invalid_channel",
				routing: []AlertRouting{
					{Severity: SeverityCritical, Channels: []string{"invalid-channel"}, Enabled: true},
				},
				errorMsg: "invalid alert channel: invalid-channel",
			},
		}

		for _, tt := range invalidRoutings {
			t.Run(tt.name, func(t *testing.T) {
				err := sam.LoadRouting(tt.routing)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			})
		}

		// Test all valid channels
		allChannelsRouting := []AlertRouting{
			{
				Severity: SeverityInfo,
				Channels: []string{"email", "webhook", "slack"},
				Enabled:  true,
			},
		}
		err = sam.LoadRouting(allChannelsRouting)
		assert.NoError(t, err)
	})

	t.Run("alert_processing", func(t *testing.T) {
		sam := NewSimpleAlertManager()

		// Setup routing
		routing := []AlertRouting{
			{
				Severity:   SeverityCritical,
				Channels:   []string{"email"},
				Recipients: []string{"admin@example.com"},
				Enabled:    true,
			},
			{
				Severity: SeverityWarning,
				Channels: []string{"slack"},
				Enabled:  false, // Disabled
			},
		}

		err := sam.LoadRouting(routing)
		require.NoError(t, err)

		// Test successful alert processing
		context := AlertContext{
			AlertID:     "test-alert-1",
			Service:     "database",
			Severity:    SeverityCritical,
			Message:     "Database is down",
			Timestamp:   time.Now(),
			Source:      "beacon-agent",
			Environment: "production",
			Tags:        map[string]string{"env": "prod"},
		}

		err = sam.ProcessAlert(context)
		assert.NoError(t, err)

		// Verify alert was stored
		activeAlert, err := sam.GetAlertStatus("test-alert-1")
		assert.NoError(t, err)
		assert.Equal(t, "test-alert-1", activeAlert.AlertID)
		assert.Equal(t, "database", activeAlert.Context.Service)
		assert.Equal(t, SeverityCritical, activeAlert.Context.Severity)
		assert.False(t, activeAlert.Acknowledged)
		assert.False(t, activeAlert.Resolved)

		// Test disabled routing
		disabledContext := AlertContext{
			AlertID:   "test-alert-2",
			Service:   "api",
			Severity:  SeverityWarning,
			Message:   "API is slow",
			Timestamp: time.Now(),
			Source:    "beacon-agent",
		}

		err = sam.ProcessAlert(disabledContext)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no routing configured for severity: warning")

		// Test unknown severity
		unknownContext := AlertContext{
			AlertID:   "test-alert-3",
			Service:   "unknown",
			Severity:  "unknown",
			Message:   "Unknown issue",
			Timestamp: time.Now(),
			Source:    "beacon-agent",
		}

		err = sam.ProcessAlert(unknownContext)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no routing configured for severity: unknown")
	})

	t.Run("cooldown_functionality", func(t *testing.T) {
		sam := NewSimpleAlertManager()

		routing := []AlertRouting{
			{
				Severity:   SeverityCritical,
				Channels:   []string{"email"},
				Recipients: []string{"admin@example.com"},
				Enabled:    true,
			},
		}

		err := sam.LoadRouting(routing)
		require.NoError(t, err)

		context := AlertContext{
			AlertID:   "cooldown-test-1",
			Service:   "database",
			Severity:  SeverityCritical,
			Message:   "Database is down",
			Timestamp: time.Now(),
			Source:    "beacon-agent",
		}

		// First alert should succeed
		err = sam.ProcessAlert(context)
		assert.NoError(t, err)

		// Second alert with same service:severity should be in cooldown
		context.AlertID = "cooldown-test-2"
		err = sam.ProcessAlert(context)
		assert.NoError(t, err) // No error, but silently ignored due to cooldown

		// Verify only first alert exists
		_, err = sam.GetAlertStatus("cooldown-test-1")
		assert.NoError(t, err)

		_, err = sam.GetAlertStatus("cooldown-test-2")
		assert.Error(t, err) // Should not exist due to cooldown
	})

	t.Run("alert_management", func(t *testing.T) {
		sam := NewSimpleAlertManager()

		routing := []AlertRouting{
			{
				Severity:   SeverityCritical,
				Channels:   []string{"email"},
				Recipients: []string{"admin@example.com"},
				Enabled:    true,
			},
		}

		err := sam.LoadRouting(routing)
		require.NoError(t, err)

		// Process test alerts
		contexts := []AlertContext{
			{
				AlertID:   "alert-1",
				Service:   "database",
				Severity:  SeverityCritical,
				Message:   "Database is down",
				Timestamp: time.Now(),
				Source:    "beacon-agent",
			},
			{
				AlertID:   "alert-2",
				Service:   "api",
				Severity:  SeverityCritical,
				Message:   "API is down",
				Timestamp: time.Now(),
				Source:    "beacon-agent",
			},
		}

		for _, context := range contexts {
			err = sam.ProcessAlert(context)
			require.NoError(t, err)
		}

		// Test GetActiveAlerts
		alerts := sam.GetActiveAlerts()
		assert.Equal(t, 2, len(alerts))
		assert.Contains(t, alerts, "alert-1")
		assert.Contains(t, alerts, "alert-2")

		// Test defensive copying
		alerts["alert-3"] = &ActiveAlert{}
		originalAlerts := sam.GetActiveAlerts()
		assert.Equal(t, 2, len(originalAlerts))
		assert.NotContains(t, originalAlerts, "alert-3")

		// Test acknowledgment
		err = sam.AcknowledgeAlert("alert-1", "admin")
		assert.NoError(t, err)

		status, err := sam.GetAlertStatus("alert-1")
		require.NoError(t, err)
		assert.True(t, status.Acknowledged)
		assert.Equal(t, "admin", status.AcknowledgedBy)
		assert.False(t, status.AcknowledgedAt.IsZero())

		// Test acknowledgment of non-existent alert
		err = sam.AcknowledgeAlert("non-existent", "admin")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "alert non-existent not found")

		// Test resolution
		err = sam.ResolveAlert("alert-1")
		assert.NoError(t, err)

		status, err = sam.GetAlertStatus("alert-1")
		require.NoError(t, err)
		assert.True(t, status.Resolved)
		assert.False(t, status.ResolvedAt.IsZero())

		// Test resolution of non-existent alert
		err = sam.ResolveAlert("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "alert non-existent not found")

		// Test GetAlertStatus for non-existent alert
		_, err = sam.GetAlertStatus("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "alert non-existent not found")
	})

	t.Run("channel_routing", func(t *testing.T) {
		sam := NewSimpleAlertManager()

		context := AlertContext{
			AlertID:   "channel-test",
			Service:   "database",
			Severity:  SeverityCritical,
			Message:   "Database is down",
			Timestamp: time.Now(),
			Source:    "beacon-agent",
		}

		recipients := []string{"admin@example.com"}

		// Test all valid channels
		validChannels := []string{"email", "slack", "webhook"}
		for _, channel := range validChannels {
			err := sam.sendToChannel(channel, recipients, context)
			assert.NoError(t, err, "Channel %s should be valid", channel)
		}

		// Test unknown channel
		err := sam.sendToChannel("unknown", recipients, context)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown alert channel: unknown")
	})

	t.Run("concurrency_safety", func(t *testing.T) {
		sam := NewSimpleAlertManager()

		routing := []AlertRouting{
			{
				Severity:   SeverityCritical,
				Channels:   []string{"email"},
				Recipients: []string{"admin@example.com"},
				Enabled:    true,
			},
		}

		err := sam.LoadRouting(routing)
		require.NoError(t, err)

		var wg sync.WaitGroup
		numGoroutines := 10
		numOperationsPerGoroutine := 10

		// Concurrent ProcessAlert operations
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < numOperationsPerGoroutine; j++ {
					context := AlertContext{
						AlertID:   fmt.Sprintf("alert-%d-%d", goroutineID, j),
						Service:   fmt.Sprintf("service-%d", goroutineID),
						Severity:  SeverityCritical,
						Message:   "Test message",
						Timestamp: time.Now(),
						Source:    "test",
					}
					sam.ProcessAlert(context)
				}
			}(i)
		}

		// Concurrent GetActiveAlerts operations
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < numOperationsPerGoroutine; j++ {
					sam.GetActiveAlerts()
				}
			}()
		}

		wg.Wait()

		// Verify final state
		alerts := sam.GetActiveAlerts()
		assert.True(t, len(alerts) > 0)
	})

	t.Run("data_structures", func(t *testing.T) {
		// Test AlertSeverity constants
		assert.Equal(t, AlertSeverity("critical"), SeverityCritical)
		assert.Equal(t, AlertSeverity("warning"), SeverityWarning)
		assert.Equal(t, AlertSeverity("info"), SeverityInfo)

		// Test AlertContext fields
		now := time.Now()
		tags := map[string]string{"env": "prod", "tier": "db"}

		context := AlertContext{
			AlertID:     "test-123",
			Service:     "database",
			Severity:    SeverityCritical,
			Message:     "Connection failed",
			Timestamp:   now,
			Tags:        tags,
			Source:      "beacon-agent",
			Environment: "production",
		}

		assert.Equal(t, "test-123", context.AlertID)
		assert.Equal(t, "database", context.Service)
		assert.Equal(t, SeverityCritical, context.Severity)
		assert.Equal(t, "Connection failed", context.Message)
		assert.Equal(t, now, context.Timestamp)
		assert.Equal(t, tags, context.Tags)
		assert.Equal(t, "beacon-agent", context.Source)
		assert.Equal(t, "production", context.Environment)

		// Test ActiveAlert fields
		routing := &AlertRouting{
			Severity: SeverityCritical,
			Channels: []string{"email"},
			Enabled:  true,
		}

		alert := ActiveAlert{
			AlertID:        "test-123",
			Context:        context,
			Routing:        routing,
			SentAt:         now,
			BackupSentAt:   now.Add(time.Hour),
			Acknowledged:   true,
			AcknowledgedBy: "admin",
			AcknowledgedAt: now.Add(time.Minute),
			Resolved:       true,
			ResolvedAt:     now.Add(2 * time.Minute),
		}

		assert.Equal(t, "test-123", alert.AlertID)
		assert.Equal(t, context, alert.Context)
		assert.Equal(t, routing, alert.Routing)
		assert.Equal(t, now, alert.SentAt)
		assert.Equal(t, now.Add(time.Hour), alert.BackupSentAt)
		assert.True(t, alert.Acknowledged)
		assert.Equal(t, "admin", alert.AcknowledgedBy)
		assert.Equal(t, now.Add(time.Minute), alert.AcknowledgedAt)
		assert.True(t, alert.Resolved)
		assert.Equal(t, now.Add(2*time.Minute), alert.ResolvedAt)
	})
}
