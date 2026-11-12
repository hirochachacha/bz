package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// requestTimeout bounds a request; lookupTimeout one for the choices of
// fields, which some Bugzillas take minutes to list (the components of
// Red Hat's Fedora).
const (
	requestTimeout = time.Minute
	lookupTimeout  = 3 * time.Minute
)

// A server is a Bugzilla and the base of its REST API.
type server struct {
	url string // as https://bugzilla.example.org
	api string // url + "/rest" or url + "/rest.cgi"
	key string // the API key; empty reads it from keyFile
}

// knownBugzillas names Bugzillas, so that a name can stand for the URL.
var knownBugzillas = map[string]string{
	"apache":      "https://bz.apache.org/bugzilla",
	"freebsd":     "https://bugs.freebsd.org/bugzilla",
	"gcc":         "https://gcc.gnu.org/bugzilla",
	"gentoo":      "https://bugs.gentoo.org",
	"kde":         "https://bugs.kde.org",
	"kernel":      "https://bugzilla.kernel.org",
	"libreoffice": "https://bugs.documentfoundation.org",
	"mozilla":     "https://bugzilla.mozilla.org",
	"openssh":     "https://bugzilla.mindrot.org",
	"opensuse":    "https://bugzilla.opensuse.org",
	"redhat":      "https://bugzilla.redhat.com",
	"samba":       "https://bugzilla.samba.org",
	"sourceware":  "https://sourceware.org/bugzilla",
	"suse":        "https://bugzilla.suse.com",
	"webkit":      "https://bugs.webkit.org",
}

// bugzillaURL returns the URL of a Bugzilla given by its URL, its host, or
// a name of knownBugzillas.
func bugzillaURL(u string) (string, error) {
	if !strings.Contains(u, "://") {
		if known, ok := knownBugzillas[u]; ok {
			u = known
		} else if strings.Contains(u, ".") {
			u = "https://" + u
		} else {
			names := slices.Sorted(maps.Keys(knownBugzillas))
			return "", fmt.Errorf("unknown Bugzilla %q: pass its host or URL, or one of %s",
				u, strings.Join(names, ", "))
		}
	}
	pu, err := url.Parse(u)
	if err != nil || pu.Scheme != "https" || pu.Host == "" {
		return "", fmt.Errorf("bad Bugzilla URL %q: want https://host[/path]", u)
	}
	return strings.TrimSuffix(u, "/"), nil
}

// dial finds the REST API of the Bugzilla at u, a URL or a name of
// knownBugzillas: at /rest, or at /rest.cgi where the web server does not
// rewrite /rest.
func dial(u string) (*server, error) {
	u, err := bugzillaURL(u)
	if err != nil {
		return nil, err
	}
	apiFile, err := cacheDir(u)
	if err != nil {
		return nil, err
	}
	apiFile = filepath.Join(apiFile, "api")
	if b, err := os.ReadFile(apiFile); err == nil {
		return &server{url: u, api: strings.TrimSpace(string(b))}, nil
	}
	for _, api := range []string{u + "/rest", u + "/rest.cgi"} {
		s := &server{url: u, api: api}
		var v struct {
			Version string `json:"version"`
		}
		if s.call("GET", "/version", nil, nil, &v) == nil && v.Version != "" {
			if os.MkdirAll(filepath.Dir(apiFile), 0o700) == nil {
				os.WriteFile(apiFile, []byte(api+"\n"), 0o600)
			}
			return s, nil
		}
	}
	return nil, fmt.Errorf("no Bugzilla REST API at %s", u)
}

// configDir holds the API key of each Bugzilla, named by its host, and
// the URL of the default Bugzilla: $XDG_CONFIG_HOME/bz, or ~/.config/bz.
func configDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bz"), nil
}

// keyFile is where the API key of the Bugzilla is kept.
func (s *server) keyFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	pu, _ := url.Parse(s.url)
	return filepath.Join(dir, pu.Host), nil
}

func (s *server) apiKey() (string, error) {
	if s.key != "" {
		return s.key, nil
	}
	keyFile, err := s.keyFile()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(keyFile)
	if key := strings.TrimSpace(string(b)); err == nil && key != "" {
		return key, nil
	}
	return "", fmt.Errorf("no API key for %s: run bz auth login --server %s", s.url, s.url)
}

// call sends a request to the REST API and decodes its JSON result into
// result. A non-nil body is sent as JSON. The API key goes with a body, and
// with every request of a server whose key is set. It goes both in the
// X-BUGZILLA-API-KEY header and as the api_key parameter: Samba's Bugzilla
// reads only the parameter, Mozilla's only the header of a POST.
func (s *server) call(method, path string, params url.Values, body, result any) error {
	return s.callWithin(requestTimeout, method, path, params, body, result)
}

// callWithin is call, bounded by timeout.
func (s *server) callWithin(timeout time.Duration, method, path string, params url.Values, body, result any) error {
	var key string
	if body != nil || s.key != "" {
		var err error
		if key, err = s.apiKey(); err != nil {
			return err
		}
		if body == nil {
			params = maps.Clone(params)
			if params == nil {
				params = url.Values{}
			}
			params.Set("api_key", key)
		} else if body, err = withAPIKey(body, key); err != nil {
			return err
		}
	}
	u := s.api + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if key != "" {
		req.Header.Set("X-BUGZILLA-API-KEY", key)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var e struct {
		Error   flag01 `json:"error"`
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if json.Unmarshal(b, &e) != nil {
		return fmt.Errorf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}
	if e.Error {
		return &apiError{method: method, path: path, message: e.Message, code: e.Code}
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}
	return json.Unmarshal(b, result)
}

// An apiError is an error that the REST API returns.
type apiError struct {
	method, path, message string
	code                  int
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s %s: %s (code %d)", e.method, e.path, e.message, e.code)
}

// withAPIKey returns the JSON object of body with the api_key member.
func withAPIKey(body any, key string) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	m["api_key"] = key
	return m, nil
}

func (s *server) bugURL(id int) string {
	return fmt.Sprintf("%s/show_bug.cgi?id=%d", s.url, id)
}

type named struct {
	Name string `json:"name"`
}

// flag01 is a flag that Bugzilla returns as 0 or 1, or as a boolean.
type flag01 bool

func (f *flag01) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "1", "true":
		*f = true
	case "0", "false":
		*f = false
	default:
		return fmt.Errorf("bad flag %s", b)
	}
	return nil
}

// oneOrMore is a field that Bugzilla returns as a string, or as a list of
// them where a bug may have several (the components of Red Hat's).
type oneOrMore string

func (f *oneOrMore) UnmarshalJSON(b []byte) error {
	var list []string
	if json.Unmarshal(b, &list) == nil {
		*f = oneOrMore(strings.Join(list, ", "))
		return nil
	}
	return json.Unmarshal(b, (*string)(f))
}

func hasName(list []named, name string) bool {
	return slices.ContainsFunc(list, func(n named) bool { return n.Name == name })
}

func isPatch(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".patch" || ext == ".diff"
}

func (s *server) attach(id int, path, comment string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	contentType := "text/plain"
	if !utf8.Valid(b) {
		contentType = "application/octet-stream"
	}
	body := map[string]any{
		"file_name":    filepath.Base(path),
		"summary":      filepath.Base(path),
		"data":         base64.StdEncoding.EncodeToString(b),
		"content_type": contentType,
		"is_patch":     isPatch(path),
	}
	if comment != "" {
		body["comment"] = comment
	}
	// Bugzilla returns the ids of the attachments made; Mozilla's returns
	// the attachments, by id.
	var result struct {
		IDs         []any                      `json:"ids"`
		Attachments map[string]json.RawMessage `json:"attachments"`
	}
	if err := s.call("POST", fmt.Sprintf("/bug/%d/attachment", id), nil, body, &result); err != nil {
		return err
	}
	ids := slices.Sorted(maps.Keys(result.Attachments))
	for _, id := range result.IDs {
		ids = append(ids, fmt.Sprint(id))
	}
	fmt.Printf("Attached %s: attachment %s\n", filepath.Base(path), strings.Join(ids, ", "))
	return nil
}

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

// selectedURL is the Bugzilla of name, else the default, as given.
func selectedURL(name string) (string, error) {
	if name != "" {
		return name, nil
	}
	u, err := defaultURL()
	if err != nil {
		return "", err
	}
	if u == "" {
		return "", errors.New("no Bugzilla: pass --server or run bz server set-default")
	}
	return u, nil
}

func serverSetDefault(args []string) error {
	f := newFlags("server set-default [<name> | <host> | <URL>]", false)
	view := f.Bool("view", false, "print the default Bugzilla")
	f.Parse(args)
	if *view {
		if err := f.wantArgs(0, 0); err != nil {
			return err
		}
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
	if err := f.wantArgs(1, 1); err != nil {
		return err
	}
	s, err := dial(f.Arg(0))
	if err != nil {
		return err
	}
	return setDefaultURL(s.url)
}
