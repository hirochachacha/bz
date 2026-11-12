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
	page := s.url + "/userprefs.cgi?tab=apikey"
	if openBrowser(page) {
		fmt.Fprintf(os.Stderr, "Opened %s in your browser; make an API key there\n", page)
	} else {
		fmt.Fprintf(os.Stderr, "Make an API key at %s\n", page)
	}
	fmt.Fprint(os.Stderr, "API key: ")
	key, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if s.key = strings.TrimSpace(key); s.key == "" {
		return fmt.Errorf("no API key given: %v", err)
	}
	user, err := s.checkKey()
	if err != nil {
		return err
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
	fmt.Printf("Logged in to %s%s; the key is in %s\n", s.url, user, keyFile)
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

// errInvalidKey is of an API key that does not log in.
var errInvalidKey = errors.New("the API key is invalid")

// checkKey checks that the key of s logs in, and returns " as <name>"
// where the Bugzilla says whose it is. /whoami says so; a Bugzilla without
// it (Samba's) still refuses a search for the bugs of %user% to a key that
// does not log in.
func (s *server) checkKey() (string, error) {
	var user struct {
		Name string `json:"name"`
	}
	err := s.call("GET", "/whoami", nil, nil, &user)
	if err == nil {
		return " as " + user.Name, nil
	}
	// Code 32614 is of a resource that does not exist.
	if e, ok := errors.AsType[*apiError](err); !ok || e.code != 32614 {
		return "", keyError(err)
	}
	var result struct{}
	err = s.call("GET", "/bug", url.Values{
		"reporter": {"%user%"}, "limit": {"1"}, "include_fields": {"id"},
	}, nil, &result)
	return "", keyError(err)
}

// keyError returns errInvalidKey for the error of a key that does not log
// in, code 306.
func keyError(err error) error {
	if e, ok := errors.AsType[*apiError](err); ok && e.code == 306 {
		return errInvalidKey
	}
	return err
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
	user, err := s.checkKey()
	if err != nil {
		return err.Error()
	}
	return "logged in" + user
}

// authLogout forgets the API key of the Bugzilla.
func authLogout(args []string) error {
	f := newFlags("auth logout", true)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	u, err := f.urlOf()
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
