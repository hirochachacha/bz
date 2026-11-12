package main

import (
	"bufio"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// authLogin reads an API key that the user made on the Bugzilla's web page,
// checks it and stores it; with no default Bugzilla set, it makes this one
// the default. It cannot make the key: Bugzilla has no API for it, and
// logging in with a password would keep the password, or a token that
// expires, instead.
func authLogin(args []string) error {
	f := newFlags("auth login", true)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	s, err := f.serverOf()
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
	fmt.Printf("Logged in to %s; the key is in %s\n", s.url, keyFile)
	if u, err := defaultURL(); err != nil || u != "" {
		return err
	}
	return setDefaultURL(s.url)
}

// authStatus checks the API key of each Bugzilla logged in to, or of the
// one of --server.
func authStatus(args []string) error {
	f := newFlags("auth status", true)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	var urls []string
	if *f.server != "" {
		u, err := bugzillaURL(*f.server)
		if err != nil {
			return err
		}
		urls = []string{u}
	} else {
		dir, err := configDir()
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for _, e := range entries {
			if e.Name() != defaultServerFile {
				urls = append(urls, hostURL(e.Name()))
			}
		}
	}
	if len(urls) == 0 {
		return errors.New("not logged in to any Bugzilla; run bz auth login")
	}
	def, _ := defaultURL()
	for _, u := range urls {
		mark := ""
		if du, err := bugzillaURL(def); err == nil && du == u {
			mark = " (default)"
		}
		fmt.Printf("%s%s: %s\n", u, mark, keyStatus(u))
	}
	return nil
}

// hostURL returns the URL of the Bugzilla of a host logged in to: the
// well-known one there, which may have a path, else the host's root.
func hostURL(host string) string {
	for _, name := range slices.Sorted(maps.Keys(knownBugzillas)) {
		if pu, err := url.Parse(knownBugzillas[name]); err == nil && pu.Host == host {
			return knownBugzillas[name]
		}
	}
	return "https://" + host
}

// keyStatus says whether the stored key of the Bugzilla at u logs in.
func keyStatus(u string) string {
	s, err := dial(u)
	if err != nil {
		return err.Error()
	}
	if s.key, err = s.apiKey(); err != nil {
		return "no API key"
	}
	// A search for the bugs of %user% needs a valid key.
	var result struct{}
	err = s.call("GET", "/bug", url.Values{
		"reporter": {"%user%"}, "limit": {"1"}, "include_fields": {"id"},
	}, nil, &result)
	if e, ok := errors.AsType[*apiError](err); ok && e.code == 306 {
		return "the API key is invalid"
	}
	if err != nil {
		return err.Error()
	}
	return "logged in"
}

// authLogout forgets the API key of the Bugzilla.
func authLogout(args []string) error {
	f := newFlags("auth logout", true)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	u, err := selectedURL(*f.server)
	if err == nil {
		u, err = bugzillaURL(u)
	}
	if err != nil {
		return err
	}
	keyFile, err := (&server{url: u}).keyFile()
	if err != nil {
		return err
	}
	if err := os.Remove(keyFile); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("not logged in to %s", u)
	} else if err != nil {
		return err
	}
	fmt.Println("Logged out of", u)
	return nil
}
