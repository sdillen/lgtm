package export

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// templateFile is the name of the prompt template in the project
// (.lgtm/prompt.template) and in the config directory.
const templateFile = "prompt.template"

// Resolve returns the prompt template: the project-local template
// <dir>/.lgtm/prompt.template wins, then the global template in the
// config directory, then the built-in default. An existing but
// unreadable template is an error, so a broken override is never
// silently replaced.
func Resolve(dir, cfgDir string) (string, error) {
	if dir != "" {
		path := filepath.Join(dir, ".lgtm", templateFile)
		text, err := readTemplate(path)
		if err != nil {
			return "", err
		}
		if text != nil {
			return *text, nil
		}
	}
	if cfgDir != "" {
		path := filepath.Join(cfgDir, templateFile)
		text, err := readTemplate(path)
		if err != nil {
			return "", err
		}
		if text != nil {
			return *text, nil
		}
	}
	return DefaultTemplate, nil
}

// readTemplate reads the template at path; nil means the file does not
// exist.
func readTemplate(path string) (*string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read prompt template %s: %w", path, err)
	}
	text := string(data)
	return &text, nil
}
