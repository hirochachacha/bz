package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// api makes a request of the REST API, as gh api does, for what the other
// commands do not: fields go in the query of a GET and in the JSON body of
// other methods, which change bugs and so ask first. The API key goes with
// every request where one is stored.
func api(args []string) error {
	f := newFlags("api <path>", true)
	method := f.StringP("method", "X", "GET", "the method of the request")
	raw := f.StringArrayP("raw-field", "f", nil, "a field of string value, as key=value")
	typed := f.StringArrayP("field", "F", nil, "a field of typed value, as key=value: true, false, null and numbers as JSON, @file for a file's content")
	input := f.String("input", "", "a file holding the JSON body, - for standard input")
	yes := yesFlag(f)
	f.Parse(args)
	if err := f.wantArgs(1, 1); err != nil {
		return err
	}
	path := "/" + strings.TrimPrefix(f.Arg(0), "/")
	*method = strings.ToUpper(*method)

	fields := map[string]any{}
	for _, kv := range *raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return fmt.Errorf("bad -f %q: want key=value", kv)
		}
		fields[k] = v
	}
	for _, kv := range *typed {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return fmt.Errorf("bad -F %q: want key=value", kv)
		}
		value, err := typedValue(v)
		if err != nil {
			return err
		}
		fields[k] = value
	}

	s, err := f.serverOf()
	if err != nil {
		return err
	}
	if key, err := s.apiKey(); err == nil {
		s.key = key
	}
	var params url.Values
	var body any
	switch {
	case *input != "" && *method == "GET":
		return fmt.Errorf("--input takes a method that sends a body")
	case *input != "":
		b, err := readInput(*input)
		if err != nil {
			return err
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("--input: %w", err)
		}
		for k, v := range fields {
			m[k] = v
		}
		body = m
	case *method == "GET":
		params = url.Values{}
		for k, v := range fields {
			params.Set(k, fmt.Sprint(v))
		}
	default:
		body = fields
	}
	if *method != "GET" {
		b, _ := json.MarshalIndent(body, "", "  ")
		if err := confirmSend(*yes, fmt.Sprintf("%s %s%s\n%s\n", *method, s.api, path, b)); err != nil {
			return err
		}
	}

	var result json.RawMessage
	if err := s.call(*method, path, params, body, &result); err != nil {
		return err
	}
	var out bytes.Buffer
	if json.Indent(&out, result, "", "  ") != nil {
		out.Write(result)
	}
	fmt.Println(out.String())
	return nil
}

// typedValue returns the value of a typed field: JSON literals and numbers
// as they are, @file as the content of the file, else a string.
func typedValue(v string) (any, error) {
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if strings.HasPrefix(v, "@") {
		b, err := readInput(v[1:])
		return string(b), err
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n, nil
	}
	if x, err := strconv.ParseFloat(v, 64); err == nil {
		return x, nil
	}
	return v, nil
}

func readInput(file string) ([]byte, error) {
	if file == "-" {
		var b bytes.Buffer
		_, err := b.ReadFrom(os.Stdin)
		return b.Bytes(), err
	}
	return os.ReadFile(file)
}
