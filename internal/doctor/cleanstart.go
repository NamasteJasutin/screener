package doctor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// CleanStartOptions controls what CleanStart does.
type CleanStartOptions struct {
	ConfigPath string
	In         io.Reader // nil = os.Stdin
	Out        io.Writer // nil = os.Stdout
}

// CleanStart prompts the user and, if confirmed, deletes the config so screener
// regenerates a fresh default on next launch. Returns true if deletion occurred.
func CleanStart(opts CleanStartOptions) (bool, error) {
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}

	fmt.Fprint(opts.Out, "Create a clean config file? [Y/n]: ")

	sc := bufio.NewScanner(opts.In)
	sc.Scan()
	answer := strings.TrimSpace(sc.Text())

	if answer != "" && strings.ToLower(answer) != "y" {
		fmt.Fprintln(opts.Out, "Aborted.")
		return false, nil
	}

	if _, statErr := os.Stat(opts.ConfigPath); os.IsNotExist(statErr) {
		fmt.Fprintln(opts.Out, "Config file not found — nothing to delete.")
		return true, nil
	}

	if err := os.Remove(opts.ConfigPath); err != nil {
		return false, fmt.Errorf("could not delete config: %w", err)
	}
	fmt.Fprintf(opts.Out, "Config deleted: %s\n", opts.ConfigPath)
	fmt.Fprintln(opts.Out, "A fresh config will be created on next launch.")
	return true, nil
}
