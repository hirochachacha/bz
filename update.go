package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type intList []int

func (l *intList) String() string { return fmt.Sprint(*l) }

func (l *intList) Set(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("bad attachment id %q", v)
	}
	*l = append(*l, n)
	return nil
}

func cmdAttach(args []string) error {
	fs := flag.NewFlagSet("attach", flag.ExitOnError)
	comment := fs.String("comment", "", "comment to add with the attachments")
	var obsoletes intList
	fs.Var(&obsoletes, "obsoletes", "an attachment of the bug that these replace, marked obsolete; repeat for more")
	yes := fs.Bool("yes", false, "send without asking; for use without a terminal, once the user approved it")
	fs.Parse(args)
	const usage = "usage: attach [-yes] [-comment text] [-obsoletes id]... bug file..."
	if fs.NArg() < 2 {
		return errors.New(usage)
	}
	id, err := bugArg(fs.Args()[:1], 1, usage)
	if err != nil {
		return err
	}
	files := fs.Args()[1:]
	for _, file := range files {
		if fi, err := os.Stat(file); err != nil {
			return err
		} else if !fi.Mode().IsRegular() {
			return fmt.Errorf("%s is not a file", file)
		}
	}
	s, err := selectedServer()
	if err != nil {
		return err
	}
	var obsoleted []string
	for _, a := range obsoletes {
		name, err := s.attachmentOf(id, a)
		if err != nil {
			return err
		}
		obsoleted = append(obsoleted, fmt.Sprintf("obsolete: attachment %d (%s)\n", a, name))
	}

	summary := fmt.Sprintf("to %s:\n%s%s", s.bugURL(id), fileSummary(files), strings.Join(obsoleted, ""))
	if *comment != "" {
		summary += "comment: " + *comment + "\n"
	}
	if err := confirmSend(*yes, summary); err != nil {
		return err
	}
	for _, file := range files {
		if err := s.attach(id, file, *comment); err != nil {
			return err
		}
	}
	for _, a := range obsoletes {
		var result struct{}
		err := s.call("PUT", fmt.Sprintf("/bug/attachment/%d", a), nil,
			map[string]any{"is_obsolete": true}, &result)
		if err != nil {
			return fmt.Errorf("attached, but %w", err)
		}
		fmt.Printf("obsoleted attachment %d\n", a)
	}
	return nil
}

// attachmentOf returns the file name of an attachment of the bug, and an
// error for one of another bug.
func (s *server) attachmentOf(bug, attachment int) (string, error) {
	var result struct {
		Attachments map[string]struct {
			BugID    int    `json:"bug_id"`
			FileName string `json:"file_name"`
		} `json:"attachments"`
	}
	err := s.call("GET", fmt.Sprintf("/bug/attachment/%d", attachment),
		map[string][]string{"include_fields": {"bug_id,file_name"}}, nil, &result)
	if err != nil {
		return "", err
	}
	a, ok := result.Attachments[strconv.Itoa(attachment)]
	if !ok || a.BugID != bug {
		return "", fmt.Errorf("attachment %d is not of bug %d", attachment, bug)
	}
	return a.FileName, nil
}

func cmdClose(args []string) error {
	fs := flag.NewFlagSet("close", flag.ExitOnError)
	resolution := fs.String("resolution", "", "the resolution, as INVALID or WORKSFORME (required); bz field resolution lists them")
	status := fs.String("status", "RESOLVED", "the status of a closed bug, CLOSED in some Bugzillas")
	dupeOf := fs.Int("dupe-of", 0, "the bug that this duplicates, for the resolution DUPLICATE")
	comment := newTextFlags(fs, "comment")
	yes := fs.Bool("yes", false, "send without asking; for use without a terminal, once the user approved it")
	fs.Parse(args)
	id, err := bugArg(fs.Args(), 1, "close [-yes] -resolution name [-dupe-of bug] [-body text | -body-file file] bug")
	if err != nil {
		return err
	}
	if *resolution == "" {
		return errors.New("missing -resolution")
	}
	if (*resolution == "DUPLICATE") != (*dupeOf != 0) {
		return errors.New("pass -dupe-of with the resolution DUPLICATE, and only with it")
	}
	var text string
	if *comment.body != "" || *comment.file != "" {
		if text, err = comment.read(); err != nil {
			return err
		}
	}
	s, err := selectedServer()
	if err != nil {
		return err
	}
	change := map[string]any{"status": *status, "resolution": *resolution}
	if *dupeOf != 0 {
		change = map[string]any{"dupe_of": *dupeOf}
	}
	if text != "" {
		change["comment"] = map[string]string{"body": text}
	}
	for field, v := range map[string]string{"status": *status, "resolution": *resolution} {
		legal, ok, err := s.legalValues(field)
		if err != nil {
			return err
		}
		if ok && len(legal) > 0 && !hasName(legal, v) {
			return fmt.Errorf("%s takes no value %q", field, v)
		}
	}

	summary := fmt.Sprintf("close %s: %s %s\n", s.bugURL(id), *status, *resolution)
	if *dupeOf != 0 {
		summary += fmt.Sprintf("duplicate of %s\n", s.bugURL(*dupeOf))
	}
	if text != "" {
		summary += "comment:\n" + text + "\n"
	}
	if err := confirmSend(*yes, summary); err != nil {
		return err
	}
	var result struct{}
	if err := s.call("PUT", fmt.Sprintf("/bug/%d", id), nil, change, &result); err != nil {
		return err
	}
	fmt.Printf("closed %s\n", s.bugURL(id))
	return nil
}
