package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
)

// textFlags take a text from -body, or from the file of -body-file, "-"
// for standard input.
type textFlags struct {
	body, file *string
}

func newTextFlags(fs *flag.FlagSet, what string) textFlags {
	return textFlags{
		body: fs.String("body", "", "the "+what),
		file: fs.String("body-file", "", "read the "+what+" from a file, - for standard input"),
	}
}

func (t textFlags) read() (string, error) {
	switch {
	case *t.body != "" && *t.file != "":
		return "", errors.New("pass -body or -body-file, not both")
	case *t.file == "-":
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	case *t.file != "":
		b, err := os.ReadFile(*t.file)
		return string(b), err
	case *t.body != "":
		return *t.body, nil
	}
	return "", errors.New("no text: pass -body or -body-file")
}

// fieldValues are the values of -set name=value, a list where a name is
// repeated.
type fieldValues map[string][]string

func (f fieldValues) String() string { return "" }

func (f fieldValues) Set(v string) error {
	name, value, ok := strings.Cut(v, "=")
	if !ok || name == "" {
		return fmt.Errorf("want name=value, not %q", v)
	}
	f[name] = append(f[name], value)
	return nil
}

type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ", ") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// fieldNames are the names of the fields of bugs that differ from the
// names that creating a bug takes.
var fieldNames = map[string]string{
	"platform": "rep_platform",
	"severity": "bug_severity",
	"status":   "bug_status",
}

// legalValues returns the active values that the field of bugs takes, none for
// a field of free text; ok is false where the Bugzilla does not know the
// field by that name.
func (s *server) legalValues(name string) (values []named, ok bool, err error) {
	if n, ok := fieldNames[name]; ok {
		name = n
	}
	var fields struct {
		Fields []struct {
			Values []choice `json:"values"`
		} `json:"fields"`
	}
	if err := s.lookup("/field/bug/"+url.PathEscape(name), nil, &fields); err != nil {
		// Code 51 is of a field that does not exist.
		if e, ok := errors.AsType[*apiError](err); ok && e.code == 51 {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(fields.Fields) == 0 {
		return nil, false, nil
	}
	return activeNames(fields.Fields[0].Values), true, nil
}

// A choice is a value that a field takes; Bugzilla keeps those no longer
// to be taken, as old versions, inactive.
type choice struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active"`
}

func (c choice) active() bool { return c.IsActive == nil || *c.IsActive }

// activeNames returns the names of the active choices.
func activeNames(choices []choice) []named {
	var names []named
	for _, c := range choices {
		if c.active() {
			names = append(names, named{Name: c.Name})
		}
	}
	return names
}

// productChoices returns the components or the versions of a product,
// with their descriptions. They are two requests, as some Bugzillas list
// the versions at once and the components never (Red Hat's Fedora).
func (s *server) productChoices(product, kind string) ([]choice, error) {
	var products struct {
		Products []map[string][]choice `json:"products"`
	}
	err := s.lookup("/product", url.Values{
		"names":          {product},
		"include_fields": {kind + ".name," + kind + ".description," + kind + ".is_active"},
	}, &products)
	if err != nil {
		return nil, err
	}
	if len(products.Products) == 0 {
		return nil, fmt.Errorf("unknown product %q", product)
	}
	return products.Products[0][kind], nil
}

// cmdProduct lists the products that take new bugs, or the active
// components and versions of one.
func cmdProduct(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: product [name]")
	}
	s, err := selectedServer()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		var products struct {
			Products []choice `json:"products"`
		}
		err := s.lookup("/product", url.Values{
			"type":           {"enterable"},
			"include_fields": {"name,description,is_active"},
		}, &products)
		if err != nil {
			return err
		}
		for _, p := range products.Products {
			if p.active() {
				fmt.Printf("%s\t%s\n", p.Name, oneLine(p.Description))
			}
		}
		return nil
	}
	versions, err := s.productChoices(args[0], "versions")
	if err != nil {
		return err
	}
	for _, v := range activeNames(versions) {
		fmt.Println("version:", v.Name)
	}
	components, err := s.productChoices(args[0], "components")
	if err != nil {
		return fmt.Errorf("components not listed: %w", err)
	}
	for _, c := range components {
		if c.active() {
			fmt.Printf("component: %s\t%s\n", c.Name, oneLine(c.Description))
		}
	}
	return nil
}

// oneLine returns the text of a description of HTML as one line.
func oneLine(html string) string {
	return strings.Join(strings.Fields(html), " ")
}

func cmdField(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: field name")
	}
	s, err := selectedServer()
	if err != nil {
		return err
	}
	values, ok, err := s.legalValues(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unknown field %q", args[0])
	}
	if len(values) == 0 {
		fmt.Println("(free text)")
	}
	for _, v := range values {
		fmt.Println(v.Name)
	}
	return nil
}

// checkBug checks the product, component and version of a new bug, and the
// values of other fields where the Bugzilla lists them.
func (s *server) checkBug(bug map[string]any) error {
	var problems []string
	product := bug["product"].(string)
	versions, err := s.productChoices(product, "versions")
	if err != nil {
		return err
	}
	if v := bug["version"].(string); !hasName(activeNames(versions), v) {
		problems = append(problems, fmt.Sprintf("product %q takes no version %q", product, v))
	}
	// A Bugzilla that cannot list the components still checks the one of
	// the bug when it is created.
	if components, err := s.productChoices(product, "components"); err != nil {
		fmt.Fprintf(os.Stderr, "bz: component not checked: %v\n", err)
	} else if c := bug["component"].(string); !hasName(activeNames(components), c) {
		problems = append(problems, fmt.Sprintf("product %q takes no component %q", product, c))
	}
	for _, name := range slices.Sorted(maps.Keys(bug)) {
		switch name {
		case "product", "component", "version", "summary", "description":
			continue
		}
		legal, ok, err := s.legalValues(name)
		if err != nil {
			return err
		}
		if !ok || len(legal) == 0 {
			continue
		}
		values, isList := bug[name].([]string)
		if !isList {
			values = []string{bug[name].(string)}
		}
		for _, v := range values {
			if !hasName(legal, v) {
				problems = append(problems, fmt.Sprintf("%s takes no value %q", name, v))
			}
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func cmdCreate(args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	productName := fs.String("product", "", "the product (required)")
	component := fs.String("component", "", "the component (required)")
	version := fs.String("version", "", "the version (required)")
	summary := fs.String("summary", "", "the summary line (required)")
	description := newTextFlags(fs, "description")
	set := fieldValues{}
	fs.Var(set, "set", "another field, as name=value (op_sys=Linux); repeat a name for a list")
	var files stringList
	fs.Var(&files, "attach", "a file to attach, .patch and .diff as patches; repeat for more")
	dryRun := fs.Bool("dry-run", false, "check the bug and print it; send nothing")
	yes := fs.Bool("yes", false, "send without asking; for use without a terminal, once the user approved it")
	fs.Parse(args)
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	var missing []string
	for name, v := range map[string]string{"product": *productName, "component": *component,
		"version": *version, "summary": *summary} {
		if v == "" {
			missing = append(missing, "-"+name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return errors.New("missing " + strings.Join(missing, ", "))
	}
	text, err := description.read()
	if err != nil {
		return err
	}
	bug := map[string]any{
		"product":     *productName,
		"component":   *component,
		"version":     *version,
		"summary":     *summary,
		"description": text,
	}
	for name, values := range set {
		if _, ok := bug[name]; ok {
			return fmt.Errorf("pass %s with -%s, not -set", name, name)
		}
		if len(values) == 1 {
			bug[name] = values[0]
		} else {
			bug[name] = values
		}
	}
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
	plan.WriteString(fileSummary(files))
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
	fmt.Println("created", s.bugURL(created.ID))
	for _, file := range files {
		if err := s.attach(created.ID, file, ""); err != nil {
			return fmt.Errorf("bug %d exists, but %w; attach the rest with bz attach", created.ID, err)
		}
	}
	return nil
}

func cmdComment(args []string) error {
	fs := flag.NewFlagSet("comment", flag.ExitOnError)
	comment := newTextFlags(fs, "comment")
	yes := fs.Bool("yes", false, "send without asking; for use without a terminal, once the user approved it")
	fs.Parse(args)
	id, err := bugArg(fs.Args(), 1, "comment [-yes] (-body text | -body-file file) bug")
	if err != nil {
		return err
	}
	text, err := comment.read()
	if err != nil {
		return err
	}
	s, err := selectedServer()
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
	fmt.Printf("comment %d on %s\n", result.ID, s.bugURL(id))
	return nil
}
