package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// InstallViaPM runs the package manager install command for tool, streaming
// output to w so the user can see progress. Returns an error if the command
// fails or the package name is not known for this manager.
func InstallViaPM(pm PackageManager, tool Tool, w io.Writer) error {
	pkg, ok := tool.Packages[pm.Name]
	if !ok {
		return fmt.Errorf("no package mapping for %s via %s", tool.Name, pm.Name)
	}

	args := pm.InstallArgs(pkg)
	binary := pm.Binary

	if pm.NeedsSudo {
		// Prepend sudo on non-Windows systems.
		if _, err := exec.LookPath("sudo"); err == nil {
			args = append([]string{binary}, args...)
			binary = "sudo"
		}
		// If sudo is absent (e.g. container running as root) just run directly.
	}

	cmd := exec.Command(binary, args...)
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Stdin = os.Stdin // allow interactive sudo password prompt
	return cmd.Run()
}
