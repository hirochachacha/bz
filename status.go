package main

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

type statusBug struct {
	ID             int       `json:"id"`
	Status         string    `json:"status"`
	Product        string    `json:"product"`
	Component      oneOrMore `json:"component"`
	Summary        string    `json:"summary"`
	LastChangeTime string    `json:"last_change_time"`
}

// cmdStatus lists the open bugs of the user of the API key: those they
// reported, those assigned to them and those they follow, the last
// changed first.
func cmdStatus(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: status")
	}
	s, err := selectedServer()
	if err != nil {
		return err
	}
	// The key logs in, so that %user% is its user.
	if s.key, err = s.apiKey(); err != nil {
		return err
	}
	for i, section := range []struct{ title, field string }{
		{"Reported by you", "reporter"},
		{"Assigned to you", "assigned_to"},
		{"CC'd to you", "cc"},
	} {
		var result struct {
			Bugs []statusBug `json:"bugs"`
		}
		err := s.call("GET", "/bug", url.Values{
			section.field:    {"%user%"},
			"resolution":     {"---"},
			"include_fields": {"id,status,product,component,summary,last_change_time"},
		}, nil, &result)
		if err != nil {
			return err
		}
		bugs := result.Bugs
		slices.SortFunc(bugs, func(a, b statusBug) int {
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
			changed, _, _ := strings.Cut(b.LastChangeTime, "T")
			fmt.Printf("  %d\t%s\t%s\t%s / %s\t%s\n", b.ID, b.Status, changed, b.Product, b.Component, b.Summary)
		}
	}
	return nil
}
