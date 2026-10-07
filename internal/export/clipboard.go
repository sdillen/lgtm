package export

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ttyWriter is what the OSC 52 terminal write needs.
type ttyWriter interface {
	io.Writer
	Close() error
}

// openTTY opens the terminal for the OSC 52 escape; a variable so tests
// can simulate a session without a terminal.
var openTTY = func() (ttyWriter, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	return tty, err
}

// fallbackTools are the local clipboard tools tried when OSC 52 is not
// available, in order; a variable so tests can stub them.
var fallbackTools = []string{"wl-copy", "xclip", "pbcopy"}

// Copy writes text to the clipboard: OSC 52 to the terminal when there
// is one (works over SSH and in multiplexers), falling back to
// wl-copy, xclip or pbcopy.
func Copy(text string) error {
	if tty, err := openTTY(); err == nil {
		// The write decides the outcome; closing is best effort.
		defer func() { _ = tty.Close() }()
		if err := writeOSC52(tty, text); err == nil {
			return nil
		}
	}
	for _, tool := range fallbackTools {
		if err := copyVia(tool, text); err == nil {
			return nil
		}
	}
	return errors.New("no clipboard available: OSC 52 failed and neither wl-copy, xclip nor pbcopy is installed")
}

// writeOSC52 writes text as the OSC 52 clipboard escape: the clipboard
// selection c, base64 encoded, terminated with BEL.
func writeOSC52(w io.Writer, text string) error {
	_, err := fmt.Fprintf(w, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))
	return err
}

// copyVia runs a local clipboard tool with text on its stdin.
func copyVia(tool, text string) error {
	path, err := exec.LookPath(tool)
	if err != nil {
		return err
	}
	var args []string
	if tool == "xclip" {
		args = []string{"-selection", "clipboard"}
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = strings.NewReader(text)
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s: %w (%s)", tool, err, msg)
		}
		return fmt.Errorf("%s: %w", tool, err)
	}
	return nil
}
