package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed SKILL.md
var skillText []byte

const skillUsage = `usage: bz skill show
       bz skill install [-global] [-yes] [-f]

  show      print the skill
  install   install the skill into ./.agents/skills/bugzilla`

func cmdSkill(args []string) error {
	if len(args) == 0 {
		return errors.New(skillUsage)
	}
	switch args[0] {
	case "show":
		_, err := os.Stdout.Write(skillText)
		return err
	case "install":
		return installSkill(args[1:])
	}
	return errors.New(skillUsage)
}

func installSkill(args []string) error {
	fs := flag.NewFlagSet("skill install", flag.ExitOnError)
	global := fs.Bool("global", false, "install into ~/.agents/skills/bugzilla")
	yes := fs.Bool("yes", false, "also create the .claude/skills/bugzilla symlink")
	force := fs.Bool("f", false, "replace an existing install or symlink")
	fs.Parse(args)
	if fs.NArg() > 0 {
		return errors.New(skillUsage)
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
		return fmt.Errorf("%s exists; pass -f to replace it", file)
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
			return fmt.Errorf("%s exists; pass -f to replace it", link)
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
