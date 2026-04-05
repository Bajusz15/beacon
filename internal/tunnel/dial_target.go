package tunnel

// DialTarget is the HTTP(S) service to forward tunnel traffic to (from local config).
type DialTarget struct {
	Protocol string // http or https
	Host     string
	Port     int
}

func (d DialTarget) wsScheme() string {
	if d.Protocol == "https" {
		return "wss"
	}
	return "ws"
}
