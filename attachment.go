package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

// attachmentDownload saves an attachment under its file name, or the name
// of --output, as gh release download does.
func attachmentDownload(args []string) error {
	f := newFlags("attachment download <attachment>", true)
	output := f.StringP("output", "O", "", "the file to write, - for standard output (default: the attachment's file name)")
	clobber := f.Bool("clobber", false, "overwrite a file that exists")
	f.Parse(args)
	if err := f.wantArgs(1, 1); err != nil {
		return err
	}
	id, err := strconv.Atoi(f.Arg(0))
	if err != nil || id <= 0 {
		return fmt.Errorf("bad attachment id %q", f.Arg(0))
	}
	s, err := f.serverOf()
	if err != nil {
		return err
	}
	if key, err := s.apiKey(); err == nil {
		s.key = key
	}
	var result struct {
		Attachments map[string]struct {
			FileName string `json:"file_name"`
			Data     string `json:"data"`
		} `json:"attachments"`
	}
	err = s.call("GET", fmt.Sprintf("/bug/attachment/%d", id),
		url.Values{"include_fields": {"file_name,data"}}, nil, &result)
	if err != nil {
		return err
	}
	a, ok := result.Attachments[strconv.Itoa(id)]
	if !ok {
		return fmt.Errorf("no attachment %d", id)
	}
	data, err := base64.StdEncoding.DecodeString(a.Data)
	if err != nil {
		return fmt.Errorf("attachment %d: %w", id, err)
	}
	if *output == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	file := *output
	if file == "" {
		// The name comes from the server: keep it to a name of this
		// directory.
		file = filepath.Base(a.FileName)
		if file == "." || file == ".." || file == string(filepath.Separator) || file == "" {
			return fmt.Errorf("attachment %d has no usable file name %q; pass --output", id, a.FileName)
		}
	}
	flag := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if *clobber {
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	w, err := os.OpenFile(file, flag, 0o644)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s exists; pass --clobber to overwrite it", file)
	}
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	fmt.Printf("Saved attachment %d to %s (%d bytes)\n", id, file, len(data))
	return nil
}
