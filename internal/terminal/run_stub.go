//go:build !unix

package terminal

import "fmt"

// RunSession is only implemented on Unix-like systems.
func RunSession(cfg RunConfig) error {
	return fmt.Errorf("remote terminal is not supported on this operating system")
}
