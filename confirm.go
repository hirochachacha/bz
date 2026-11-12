package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// confirmSend asks before sending what summary shows to a public tracker:
// on a terminal it shows the summary and asks; elsewhere, as for an agent,
// it sends only with -yes, which says that the user has seen and approved
// it.
func confirmSend(yes bool, summary string) error {
	if yes {
		return nil
	}
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return errors.New("not sent: no terminal to confirm on; pass -yes once the user approved it")
	}
	fmt.Fprint(os.Stderr, summary)
	fmt.Fprint(os.Stderr, "Send? [y/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	return errors.New("not sent")
}

// fileSummary describes the files to attach.
func fileSummary(files []string) string {
	var b strings.Builder
	for _, file := range files {
		kind := "file"
		if isPatch(file) {
			kind = "patch"
		}
		size := "?"
		if fi, err := os.Stat(file); err == nil {
			size = fmt.Sprint(fi.Size())
		}
		fmt.Fprintf(&b, "attach: %s (%s, %s bytes)\n", filepath.Base(file), kind, size)
	}
	return b.String()
}
