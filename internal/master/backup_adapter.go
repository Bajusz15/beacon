package master

import "time"

// backupEventAdapter adapts EventLog to backup.EventAppender.
type backupEventAdapter struct {
	el *EventLog
}

func (a *backupEventAdapter) AppendBackupEvent(message string, durationMs int64) {
	a.el.Append(Event{
		Timestamp:  time.Now(),
		Type:       EventBackup,
		Message:    message,
		DurationMs: durationMs,
	})
}
