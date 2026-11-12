package main

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

// A listedBug is a bug as the lists print it.
type listedBug struct {
	ID             int       `json:"id"`
	Status         string    `json:"status"`
	Resolution     string    `json:"resolution"`
	Product        string    `json:"product"`
	Component      oneOrMore `json:"component"`
	Summary        string    `json:"summary"`
	LastChangeTime string    `json:"last_change_time"`
}

const listedFields = "id,status,resolution,product,component,summary,last_change_time"

func (s *server) searchBugs(params url.Values) ([]listedBug, error) {
	params.Set("include_fields", listedFields)
	var result struct {
		Bugs []listedBug `json:"bugs"`
	}
	if err := s.call("GET", "/bug", params, nil, &result); err != nil {
		return nil, err
	}
	return result.Bugs, nil
}

func printBug(indent string, b listedBug) {
	status := strings.TrimSpace(b.Status + " " + b.Resolution)
	changed, _, _ := strings.Cut(b.LastChangeTime, "T")
	fmt.Printf("%s%d\t%s\t%s\t%s / %s\t%s\n", indent, b.ID, status, changed, b.Product, b.Component, b.Summary)
}

// userParam sets a field of people to a login, or to the user of the API
// key for @me.
func (s *server) userParam(params url.Values, field, login string) error {
	if login == "" {
		return nil
	}
	if login == "@me" {
		// The key logs in, so that %user% is its user.
		key, err := s.apiKey()
		if err != nil {
			return err
		}
		s.key = key
		login = "%user%"
	}
	params.Set(field, login)
	return nil
}

// bugList lists bugs by fields, by words, or both. Words alone go to
// quicksearch; with fields they match the summary, as quicksearch ignores
// the other criteria of a search.
func bugList(args []string) error {
	f := newFlags("bug list", true)
	author := f.StringP("author", "A", "", "the reporter's login, @me for you")
	assignee := f.StringP("assignee", "a", "", "the assignee's login, @me for you")
	product := f.String("product", "", "the product")
	component := f.String("component", "", "the component")
	state := f.StringP("state", "s", "open", "the bugs to list: open, closed or all")
	search := f.StringP("search", "S", "", "words to search for")
	limit := f.IntP("limit", "L", 30, "the most bugs to list")
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	if *state != "open" && *state != "closed" && *state != "all" {
		return fmt.Errorf("bad --state %q: want open, closed or all", *state)
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	params := url.Values{"limit": {strconv.Itoa(*limit)}}
	if *product != "" {
		params.Set("product", *product)
	}
	if *component != "" {
		params.Set("component", *component)
	}
	if err := s.userParam(params, "reporter", *author); err != nil {
		return err
	}
	if err := s.userParam(params, "assigned_to", *assignee); err != nil {
		return err
	}
	// Quicksearch takes the open bugs, or all with ALL; it cannot take the
	// closed only.
	if len(params) == 1 && *search != "" && *state != "closed" {
		q := *search
		if *state == "all" {
			q = "ALL " + q
		}
		params.Set("quicksearch", q)
	} else {
		if *search != "" {
			params.Set("short_desc", *search)
			params.Set("short_desc_type", "allwordssubstr")
		}
		switch *state {
		case "open":
			params.Set("resolution", "---")
		case "closed":
			params.Set("f1", "resolution")
			params.Set("o1", "notequals")
			params.Set("v1", "---")
		}
	}
	bugs, err := s.searchBugs(params)
	if err != nil {
		return err
	}
	for _, b := range bugs {
		printBug("", b)
	}
	return nil
}

// bugStatus lists the open bugs of the user of the API key: those they
// reported, those assigned to them and those they follow, the last
// changed first.
func bugStatus(args []string) error {
	f := newFlags("bug status", true)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	for i, section := range []struct{ title, field string }{
		{"Reported by you", "reporter"},
		{"Assigned to you", "assigned_to"},
		{"CC'd to you", "cc"},
	} {
		params := url.Values{"resolution": {"---"}}
		if err := s.userParam(params, section.field, "@me"); err != nil {
			return err
		}
		bugs, err := s.searchBugs(params)
		if err != nil {
			return err
		}
		slices.SortFunc(bugs, func(a, b listedBug) int {
			return strings.Compare(b.LastChangeTime, a.LastChangeTime)
		})
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(section.title)
		if len(bugs) == 0 {
			fmt.Println("  none")
		}
		for _, b := range bugs {
			printBug("  ", b)
		}
	}
	return nil
}

func bugView(args []string) error {
	f := newFlags("bug view <bug>", true)
	withComments := f.BoolP("comments", "c", false, "show all the comments, not the description only")
	f.Parse(args)
	if err := f.wantArgs(1, 1); err != nil {
		return err
	}
	id, err := bugID(f.Arg(0))
	if err != nil {
		return err
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	var bugs struct {
		Bugs []struct {
			Summary    string    `json:"summary"`
			Product    string    `json:"product"`
			Component  oneOrMore `json:"component"`
			Version    oneOrMore `json:"version"`
			Status     string    `json:"status"`
			Resolution string    `json:"resolution"`
			Creator    string    `json:"creator"`
			AssignedTo string    `json:"assigned_to"`
		} `json:"bugs"`
	}
	if err := s.call("GET", fmt.Sprintf("/bug/%d", id), nil, nil, &bugs); err != nil {
		return err
	}
	b := bugs.Bugs[0]
	fmt.Printf("%s\n%s\n", b.Summary, s.bugURL(id))
	fmt.Printf("%s / %s / %s\n", b.Product, b.Component, b.Version)
	fmt.Printf("%s; reported by %s, assigned to %s\n",
		strings.TrimSpace(b.Status+" "+b.Resolution), b.Creator, b.AssignedTo)

	var attachments struct {
		Bugs map[string][]struct {
			ID           int    `json:"id"`
			FileName     string `json:"file_name"`
			ContentType  string `json:"content_type"`
			IsPatch      flag01 `json:"is_patch"`
			IsObsolete   flag01 `json:"is_obsolete"`
			Creator      string `json:"creator"`
			CreationTime string `json:"creation_time"`
		} `json:"bugs"`
	}
	err = s.call("GET", fmt.Sprintf("/bug/%d/attachment", id),
		url.Values{"exclude_fields": {"data"}}, nil, &attachments)
	if err != nil {
		return err
	}
	for _, a := range attachments.Bugs[strconv.Itoa(id)] {
		kind := a.ContentType
		if a.IsPatch {
			kind = "patch"
		}
		if a.IsObsolete {
			kind += ", obsolete"
		}
		fmt.Printf("attachment %d: %s (%s) %s %s\n", a.ID, a.FileName, kind, a.Creator, a.CreationTime)
	}

	var comments struct {
		Bugs map[string]struct {
			Comments []struct {
				Count   int    `json:"count"`
				Creator string `json:"creator"`
				Time    string `json:"time"`
				Text    string `json:"text"`
			} `json:"comments"`
		} `json:"bugs"`
	}
	if err := s.call("GET", fmt.Sprintf("/bug/%d/comment", id), nil, nil, &comments); err != nil {
		return err
	}
	list := comments.Bugs[strconv.Itoa(id)].Comments
	for _, c := range list {
		if c.Count > 0 && !*withComments {
			break
		}
		fmt.Printf("\n--- comment %d %s %s\n%s\n", c.Count, c.Creator, c.Time, c.Text)
	}
	if n := len(list) - 1; n > 0 && !*withComments {
		fmt.Printf("\n%d more comments; --comments shows them\n", n)
	}
	return nil
}

// textFlags take a text from --body, or from the file of --body-file, "-"
// for standard input.
type textFlags struct {
	body, file *string
}

func newTextFlags(f *flags, what string) textFlags {
	return textFlags{
		body: f.StringP("body", "b", "", "the "+what),
		file: f.StringP("body-file", "F", "", "read the "+what+" from a file, - for standard input"),
	}
}

func (t textFlags) read() (string, error) {
	switch {
	case *t.body != "" && *t.file != "":
		return "", errors.New("pass --body or --body-file, not both")
	case *t.file == "-":
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	case *t.file != "":
		b, err := os.ReadFile(*t.file)
		return string(b), err
	case *t.body != "":
		return *t.body, nil
	}
	return "", errors.New("no text: pass --body or --body-file")
}

// yesFlag is the flag that sends without asking.
func yesFlag(f *flags) *bool {
	return f.Bool("yes", false, "send without asking; for use without a terminal, once the user approved it")
}

// checkFiles checks that the files to attach are files.
func checkFiles(files []string) error {
	for _, file := range files {
		if fi, err := os.Stat(file); err != nil {
			return err
		} else if !fi.Mode().IsRegular() {
			return fmt.Errorf("%s is not a file", file)
		}
	}
	return nil
}

func bugCreate(args []string) error {
	f := newFlags("bug create", true)
	title := f.StringP("title", "t", "", "the summary of the bug (required)")
	product := f.String("product", "", "the product (required)")
	component := f.String("component", "", "the component (required)")
	version := f.String("version", "", "the version (required)")
	description := newTextFlags(f, "description")
	set := f.StringArray("set", nil, "another field, as name=value (op_sys=Linux); repeat a name for a list")
	files := f.StringArray("attach", nil, "a file to attach, .patch and .diff as patches; repeat for more")
	dryRun := f.Bool("dry-run", false, "check the bug and print it; send nothing")
	yes := yesFlag(f)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	var missing []string
	for _, r := range []struct{ flag, value string }{
		{"--title", *title}, {"--product", *product}, {"--component", *component}, {"--version", *version},
	} {
		if r.value == "" {
			missing = append(missing, r.flag)
		}
	}
	if len(missing) > 0 {
		return errors.New("missing " + strings.Join(missing, ", "))
	}
	text, err := description.read()
	if err != nil {
		return err
	}
	bug := map[string]any{
		"product":     *product,
		"component":   *component,
		"version":     *version,
		"summary":     *title,
		"description": text,
	}
	values := map[string][]string{}
	for _, v := range *set {
		name, value, ok := strings.Cut(v, "=")
		if !ok || name == "" {
			return fmt.Errorf("bad --set %q: want name=value", v)
		}
		if _, ok := bug[name]; ok {
			return fmt.Errorf("pass %s with its own flag, not --set", name)
		}
		values[name] = append(values[name], value)
	}
	for name, v := range values {
		if len(v) == 1 {
			bug[name] = v[0]
		} else {
			bug[name] = v
		}
	}
	if err := checkFiles(*files); err != nil {
		return err
	}

	s, err := f.serverOf()
	if err != nil {
		return err
	}
	if err := s.checkBug(bug); err != nil {
		return err
	}
	var plan strings.Builder
	fmt.Fprintln(&plan, "bugzilla:", s.url)
	for _, name := range slices.Sorted(maps.Keys(bug)) {
		if name != "description" {
			fmt.Fprintf(&plan, "%s: %v\n", name, bug[name])
		}
	}
	fmt.Fprintf(&plan, "description: %d bytes, %d lines\n", len(text), strings.Count(text, "\n"))
	plan.WriteString(fileSummary(*files))
	if *dryRun {
		fmt.Print(plan.String())
		return nil
	}
	if err := confirmSend(*yes, plan.String()); err != nil {
		return err
	}

	var created struct {
		ID int `json:"id"`
	}
	if err := s.call("POST", "/bug", nil, bug, &created); err != nil {
		return err
	}
	fmt.Println("Created", s.bugURL(created.ID))
	for _, file := range *files {
		if err := s.attach(created.ID, file, ""); err != nil {
			return fmt.Errorf("bug %d exists, but %w; attach the rest with bz bug attach", created.ID, err)
		}
	}
	return nil
}

func bugComment(args []string) error {
	f := newFlags("bug comment <bug>", true)
	comment := newTextFlags(f, "comment")
	yes := yesFlag(f)
	f.Parse(args)
	if err := f.wantArgs(1, 1); err != nil {
		return err
	}
	id, err := bugID(f.Arg(0))
	if err != nil {
		return err
	}
	text, err := comment.read()
	if err != nil {
		return err
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	if err := confirmSend(*yes, fmt.Sprintf("comment on %s:\n%s\n", s.bugURL(id), text)); err != nil {
		return err
	}
	var result struct {
		ID int `json:"id"`
	}
	err = s.call("POST", fmt.Sprintf("/bug/%d/comment", id), nil,
		map[string]string{"comment": text}, &result)
	if err != nil {
		return err
	}
	fmt.Printf("Comment %d on %s\n", result.ID, s.bugURL(id))
	return nil
}

func bugAttach(args []string) error {
	f := newFlags("bug attach <bug> <file>...", true)
	comment := f.StringP("comment", "c", "", "a comment to add with the attachments")
	obsoletes := f.IntSlice("obsoletes", nil, "attachments of the bug that these replace, to mark obsolete")
	yes := yesFlag(f)
	f.Parse(args)
	if err := f.wantArgs(2, -1); err != nil {
		return err
	}
	id, err := bugID(f.Arg(0))
	if err != nil {
		return err
	}
	files := f.Args()[1:]
	if err := checkFiles(files); err != nil {
		return err
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	plan := fmt.Sprintf("to %s:\n%s", s.bugURL(id), fileSummary(files))
	for _, a := range *obsoletes {
		name, err := s.attachmentOf(id, a)
		if err != nil {
			return err
		}
		plan += fmt.Sprintf("obsolete: attachment %d (%s)\n", a, name)
	}
	if *comment != "" {
		plan += "comment: " + *comment + "\n"
	}
	if err := confirmSend(*yes, plan); err != nil {
		return err
	}
	for _, file := range files {
		if err := s.attach(id, file, *comment); err != nil {
			return err
		}
	}
	for _, a := range *obsoletes {
		var result struct{}
		err := s.call("PUT", fmt.Sprintf("/bug/attachment/%d", a), nil,
			map[string]any{"is_obsolete": true}, &result)
		if err != nil {
			return fmt.Errorf("attached, but %w", err)
		}
		fmt.Printf("Obsoleted attachment %d\n", a)
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
		url.Values{"include_fields": {"bug_id,file_name"}}, nil, &result)
	if err != nil {
		return "", err
	}
	a, ok := result.Attachments[strconv.Itoa(attachment)]
	if !ok || a.BugID != bug {
		return "", fmt.Errorf("attachment %d is not of bug %d", attachment, bug)
	}
	return a.FileName, nil
}

func bugClose(args []string) error {
	f := newFlags("bug close <bug>", true)
	resolution := f.StringP("resolution", "r", "", "the resolution, as INVALID or WORKSFORME; bz field view resolution lists them")
	duplicateOf := f.Int("duplicate-of", 0, "the bug that this duplicates; the resolution is DUPLICATE")
	status := f.String("status", "RESOLVED", "the status of a closed bug, CLOSED in some Bugzillas")
	comment := f.StringP("comment", "c", "", "a comment to add")
	yes := yesFlag(f)
	f.Parse(args)
	if err := f.wantArgs(1, 1); err != nil {
		return err
	}
	id, err := bugID(f.Arg(0))
	if err != nil {
		return err
	}
	switch {
	case *duplicateOf != 0 && *resolution != "" && *resolution != "DUPLICATE":
		return errors.New("--duplicate-of takes the resolution DUPLICATE")
	case *duplicateOf != 0:
		*resolution = "DUPLICATE"
	case *resolution == "DUPLICATE":
		return errors.New("pass the bug it duplicates with --duplicate-of")
	case *resolution == "":
		return errors.New("missing --resolution or --duplicate-of")
	}
	s, err := f.serverOf()
	if err != nil {
		return err
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
	change := map[string]any{"status": *status, "resolution": *resolution}
	plan := fmt.Sprintf("close %s: %s %s\n", s.bugURL(id), *status, *resolution)
	if *duplicateOf != 0 {
		// Bugzilla resolves a bug as a duplicate by dupe_of alone.
		change = map[string]any{"dupe_of": *duplicateOf}
		plan += fmt.Sprintf("duplicate of %s\n", s.bugURL(*duplicateOf))
	}
	if *comment != "" {
		change["comment"] = map[string]string{"body": *comment}
		plan += "comment: " + *comment + "\n"
	}
	if err := confirmSend(*yes, plan); err != nil {
		return err
	}
	var result struct{}
	if err := s.call("PUT", fmt.Sprintf("/bug/%d", id), nil, change, &result); err != nil {
		return err
	}
	fmt.Println("Closed", s.bugURL(id))
	return nil
}

// reopenStatuses are the statuses that reopen a bug, the first that the
// Bugzilla takes being the default: REOPENED of older workflows,
// CONFIRMED of that of Bugzilla 5.
var reopenStatuses = []string{"REOPENED", "CONFIRMED"}

func bugReopen(args []string) error {
	f := newFlags("bug reopen <bug>", true)
	status := f.String("status", "", "the open status to reopen to (default: REOPENED, else CONFIRMED)")
	comment := f.StringP("comment", "c", "", "a comment to add")
	yes := yesFlag(f)
	f.Parse(args)
	if err := f.wantArgs(1, 1); err != nil {
		return err
	}
	id, err := bugID(f.Arg(0))
	if err != nil {
		return err
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	legal, ok, err := s.legalValues("status")
	if err != nil {
		return err
	}
	if *status == "" {
		for _, st := range reopenStatuses {
			if !ok || hasName(legal, st) {
				*status = st
				break
			}
		}
		if *status == "" {
			return errors.New("this Bugzilla has no REOPENED or CONFIRMED; pass --status (bz field view status)")
		}
	} else if ok && !hasName(legal, *status) {
		return fmt.Errorf("status takes no value %q", *status)
	}
	// Bugzilla clears the resolution of a bug that it opens.
	change := map[string]any{"status": *status}
	plan := fmt.Sprintf("reopen %s: %s\n", s.bugURL(id), *status)
	if *comment != "" {
		change["comment"] = map[string]string{"body": *comment}
		plan += "comment: " + *comment + "\n"
	}
	if err := confirmSend(*yes, plan); err != nil {
		return err
	}
	var result struct{}
	err = s.call("PUT", fmt.Sprintf("/bug/%d", id), nil, change, &result)
	// Code 123 is of a change of status that the workflow, or the user's
	// permissions, do not allow, as REOPENED for a reporter in Mozilla's.
	if e, ok := errors.AsType[*apiError](err); ok && e.code == 123 {
		return fmt.Errorf("%w; try another --status, as UNCONFIRMED (bz field view status)", err)
	}
	if err != nil {
		return err
	}
	fmt.Println("Reopened", s.bugURL(id))
	return nil
}
