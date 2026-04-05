package master

import "beacon/internal/logging"

// logger is the shared master-component logger. Its prefix is "[Beacon master]".
// Sub-components that need a more specific prefix (e.g. tunnels with their own ID)
// create their own logger directly.
var logger = logging.New("master")
