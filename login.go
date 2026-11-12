package main

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// cmdLogin reads an API key that the user made on the Bugzilla's web page,
// checks it and stores it, for the Bugzilla of its argument, else the one
// default; with no default set, it makes this one the default. It cannot make one: Bugzilla has no API for it,
// and logging in with a password would keep the password or a token that
// expires instead.
func cmdLogin(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: login [URL | name]")
	}
	var s *server
	var err error
	if len(args) == 1 {
		s, err = dial(args[0])
	} else {
		s, err = selectedServer()
	}
	if err != nil {
		return err
	}
	in := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stderr, "Login (email): ")
	login, err := in.ReadString('\n')
	if login = strings.TrimSpace(login); login == "" {
		return fmt.Errorf("no login given: %v", err)
	}
	fmt.Fprintf(os.Stderr, "Make an API key at %s/userprefs.cgi?tab=apikey\n", s.url)
	fmt.Fprint(os.Stderr, "API key: ")
	key, err := in.ReadString('\n')
	if s.key = strings.TrimSpace(key); s.key == "" {
		return fmt.Errorf("no API key given: %v", err)
	}
	// Bugzilla shows the email of a user only to one logged in, so it
	// coming back says that the key is valid; not all have /whoami.
	var users struct {
		Users []struct {
			Email string `json:"email"`
		} `json:"users"`
	}
	err = s.call("GET", "/user", url.Values{
		"names": {login}, "include_fields": {"email"},
	}, nil, &users)
	if err != nil {
		return err
	}
	if len(users.Users) == 0 || users.Users[0].Email == "" {
		return fmt.Errorf("the API key does not log in to %s", s.url)
	}
	keyFile, err := s.keyFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, []byte(s.key+"\n"), 0o600); err != nil {
		return err
	}
	// WriteFile keeps the mode of a file that exists.
	if err := os.Chmod(keyFile, 0o600); err != nil {
		return err
	}
	fmt.Printf("the key logs in to %s; stored in %s\n", s.url, keyFile)
	// The first Bugzilla logged in to is the default.
	if u, err := defaultURL(); err != nil || u != "" {
		return err
	}
	return setDefaultURL(s.url)
}
