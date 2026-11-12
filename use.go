package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultServerFile holds the URL of the default Bugzilla; no host is named
// so.
const defaultServerFile = "current"

// defaultURL returns the URL of the default Bugzilla, empty if none is set.
func defaultURL() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(dir, defaultServerFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(b)), err
}

// cmdUse sets the default Bugzilla, by its URL, host or name.
// Without an argument it prints the one set.
func cmdUse(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: use [name | host | URL]")
	}
	if len(args) == 0 {
		u, err := defaultURL()
		if err != nil {
			return err
		}
		if u == "" {
			return errors.New("no default Bugzilla set")
		}
		fmt.Println(u)
		return nil
	}
	s, err := dial(args[0])
	if err != nil {
		return err
	}
	return setDefaultURL(s.url)
}

// setDefaultURL makes the Bugzilla at u the default.
func setDefaultURL(u string) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, defaultServerFile), []byte(u+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Println("default Bugzilla:", u)
	return nil
}
