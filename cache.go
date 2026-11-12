package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cacheAge is how long the choices of fields are taken from the cache.
const cacheAge = 24 * time.Hour

// lookup gets the choices of fields, as products and the values of fields,
// from the cache of the Bugzilla, else from the Bugzilla, and caches what
// it gets. It caches a lookup that timed out too, so that a Bugzilla that
// cannot list a choice (the components of Red Hat's Fedora) costs the wait
// once a day.
func (s *server) lookup(path string, params url.Values, result any) error {
	file, err := s.cacheFile(path, params)
	if err != nil {
		return err
	}
	timedOut := strings.TrimSuffix(file, ".json") + ".timeout"
	if fresh(timedOut) {
		return fmt.Errorf("GET %s timed out within a day; bz cache clear to try again", path)
	}
	if fresh(file) {
		if b, err := os.ReadFile(file); err == nil && json.Unmarshal(b, result) == nil {
			return nil
		}
	}
	var raw json.RawMessage
	err = s.callWithin(lookupTimeout, "GET", path, params, nil, &raw)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// A cache that cannot be written only costs the next lookup.
	if os.MkdirAll(filepath.Dir(file), 0o700) == nil {
		if err != nil {
			os.WriteFile(timedOut, nil, 0o600)
		} else {
			os.WriteFile(file, raw, 0o600)
		}
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, result)
}

// fresh reports whether the file of the cache exists and is younger than
// cacheAge.
func fresh(file string) bool {
	fi, err := os.Stat(file)
	return err == nil && time.Since(fi.ModTime()) < cacheAge
}

// cacheDir is the directory of the cache of the Bugzilla at u, under
// $XDG_CACHE_HOME/bz, or ~/.cache/bz.
func cacheDir(u string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	pu, _ := url.Parse(u)
	return filepath.Join(dir, "bz", pu.Host+strings.ReplaceAll(pu.Path, "/", "_")), nil
}

// cacheFile is the file of the cache of a request, named by its hash.
func (s *server) cacheFile(path string, params url.Values) (string, error) {
	dir, err := cacheDir(s.url)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(s.api + path + "?" + params.Encode()))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json"), nil
}

// cacheDirOf returns the URL of the Bugzilla of the flags and the
// directory of its cache.
func (f *flags) cacheDirOf() (string, string, error) {
	u, err := f.urlOf()
	if err != nil {
		return "", "", err
	}
	dir, err := cacheDir(u)
	return u, dir, err
}

// cacheClear drops the cache of the Bugzilla, so that the next commands
// fetch the choices of fields and its REST API again.
func cacheClear(args []string) error {
	f := newFlags("cache clear", true)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	u, dir, err := f.cacheDirOf()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	fmt.Println("Dropped the cache of", u)
	return nil
}

func cacheDirectory(args []string) error {
	f := newFlags("cache dir", true)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	_, dir, err := f.cacheDirOf()
	if err != nil {
		return err
	}
	fmt.Println(dir)
	return nil
}
