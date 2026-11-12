package main

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
)

// fieldNames are the names of the fields of bugs that differ from the
// names that creating a bug takes.
var fieldNames = map[string]string{
	"platform": "rep_platform",
	"severity": "bug_severity",
	"status":   "bug_status",
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

// legalValues returns the active values that the field of bugs takes, none
// for a field of free text; ok is false where the Bugzilla does not know
// the field by that name.
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

func productList(args []string) error {
	f := newFlags("product list", true)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	var products struct {
		Products []choice `json:"products"`
	}
	err = s.lookup("/product", url.Values{
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

// productView prints the active versions and components of a product; the
// components may fail where the versions do not.
func productView(args []string) error {
	f := newFlags("product view <product>", true)
	f.Parse(args)
	if err := f.wantArgs(1, 1); err != nil {
		return err
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	versions, err := s.productChoices(f.Arg(0), "versions")
	if err != nil {
		return err
	}
	for _, v := range activeNames(versions) {
		fmt.Println("version:", v.Name)
	}
	components, err := s.productChoices(f.Arg(0), "components")
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

func fieldView(args []string) error {
	f := newFlags("field view <field>", true)
	f.Parse(args)
	if err := f.wantArgs(1, 1); err != nil {
		return err
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	values, ok, err := s.legalValues(f.Arg(0))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unknown field %q", f.Arg(0))
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
