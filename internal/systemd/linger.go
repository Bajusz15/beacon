package systemd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// EnsureLingering lets user services start at boot and survive the last logout.
// It intentionally does nothing for system services. Uninstalling Beacon must
// not disable lingering: other services belonging to this user may rely on it.
func (sm *ServiceManager) EnsureLingering() error {
	if sm.serviceType != UserService {
		return nil
	}
	uid := strconv.Itoa(os.Geteuid())
	if output, err := runLingerCommand("loginctl", "show-user", uid, "--property=Linger", "--value"); err == nil && strings.TrimSpace(string(output)) == "yes" {
		return nil
	}
	// Avoid waiting for a PolicyKit agent or a sudo password in unattended setup.
	output, err := runLingerCommand("loginctl", "--no-ask-password", "enable-linger", uid)
	if err != nil {
		output, err = runLingerCommand("sudo", "-n", "loginctl", "--no-ask-password", "enable-linger", uid)
	}
	if err != nil {
		return fmt.Errorf("cannot enable user lingering: %w (%s); run `sudo loginctl enable-linger %s`, then retry installation", err, strings.TrimSpace(string(output)), uid)
	}
	output, err = runLingerCommand("loginctl", "show-user", uid, "--property=Linger", "--value")
	if err != nil || strings.TrimSpace(string(output)) != "yes" {
		return fmt.Errorf("could not verify user lingering (output %q, error %v); run `sudo loginctl enable-linger %s`, then retry installation", strings.TrimSpace(string(output)), err, uid)
	}
	return nil
}

func runLingerCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
