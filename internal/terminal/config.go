package terminal

// RunConfig is passed to RunSession for a terminal_open command.
type RunConfig struct {
	WSURL      string
	APIKey     string
	DeviceName string
	CommandID  string
}
