package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed SKILL.md
var skillText []byte

func skillShow(args []string) error {
	f := newFlags("skill show", false)
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}
	_, err := os.Stdout.Write(skillText)
	return err
}

func skillInstall(args []string) error {
	f := newFlags("skill install", false)
	global := f.Bool("global", false, "install into ~/.agents/skills/bugzilla")
	yes := f.Bool("yes", false, "also create the .claude/skills/bugzilla symlink")
	force := f.BoolP("force", "f", false, "replace an existing install or symlink")
	f.Parse(args)
	if err := f.wantArgs(0, 0); err != nil {
		return err
	}

	root := "."
	if *global {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		root = home
	}
	dir := filepath.Join(root, ".agents", "skills", "bugzilla")
	file := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(file); err == nil && !*force {
		return fmt.Errorf("%s exists; pass --force to replace it", file)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(file, skillText, 0o644); err != nil {
		return err
	}
	fmt.Println("installed", file)
	if !*yes {
		return nil
	}

	link := filepath.Join(root, ".claude", "skills", "bugzilla")
	if _, err := os.Lstat(link); err == nil {
		if !*force {
			return fmt.Errorf("%s exists; pass --force to replace it", link)
		}
		if err := os.RemoveAll(link); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	// Relative, so that the link of a project survives a move of it.
	if err := os.Symlink(filepath.Join("..", "..", ".agents", "skills", "bugzilla"), link); err != nil {
		return err
	}
	fmt.Println("linked", link)
	return nil
}
