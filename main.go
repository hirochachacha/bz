// Command bz searches, reads and files bugs in a Bugzilla through its REST
// API, with commands in the manner of gh: bz bug list, bz bug view,
// bz auth login.
//
// A command uses the Bugzilla of --server, else the default that bz server
// set-default set, by its URL, its host or the name of a well-known one, as
// samba. The API key of a Bugzilla is read from ~/.config/bz/<host>, where
// bz auth login stores it. Only bug create, comment, attach and close
// change a bug.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/pflag"
)

type command struct {
	name, args, help string
	run              func(args []string) error
}

// A group is a group of commands, or a command of its own where run is
// set (bz api).
type group struct {
	name, help string
	commands   []command
	run        func(args []string) error
}

var groups = []group{
	{"auth", "Log in to a Bugzilla", []command{
		{"login", "", "Store an API key, read from the terminal", authLogin},
		{"status", "", "Check the stored API keys", authStatus},
		{"logout", "", "Forget the API key of a Bugzilla", authLogout},
	}, nil},
	{"bug", "Search, read and file bugs", []command{
		{"list", "", "List bugs by fields or words", bugList},
		{"status", "", "List your open bugs: reported, assigned, CC'd", bugStatus},
		{"view", "<bug>", "Show a bug and its attachments", bugView},
		{"create", "", "File a bug, with files to attach", bugCreate},
		{"comment", "<bug>", "Add a comment to a bug", bugComment},
		{"attach", "<bug> <file>...", "Attach files to a bug, obsoleting others", bugAttach},
		{"close", "<bug>", "Resolve a bug", bugClose},
		{"edit", "<bug>", "Change the fields of a bug", bugEdit},
		{"reopen", "<bug>", "Reopen a closed bug", bugReopen},
	}, nil},
	{"attachment", "Fetch the attachments of bugs", []command{
		{"download", "<attachment>", "Save an attachment to a file", attachmentDownload},
	}, nil},
	{"product", "Show the products of a Bugzilla", []command{
		{"list", "", "List the products that take new bugs", productList},
		{"view", "<product>", "Show the components and versions of a product", productView},
	}, nil},
	{"field", "Show the fields of bugs", []command{
		{"view", "<field>", "Show the values that a field takes", fieldView},
	}, nil},
	{"server", "Choose the default Bugzilla", []command{
		{"set-default", "[<name> | <host> | <URL>]", "Set or view the default Bugzilla", serverSetDefault},
	}, nil},
	{"cache", "Manage the cache of the choices of fields", []command{
		{"clear", "", "Drop the cache of a Bugzilla", cacheClear},
		{"dir", "", "Print the directory of the cache of a Bugzilla", cacheDirectory},
	}, nil},
	{"skill", "Print or install the skill for agents", []command{
		{"show", "", "Print the skill", skillShow},
		{"install", "", "Install the skill into ./.agents/skills/bugzilla", skillInstall},
	}, nil},
	{"api", "Make a request of the REST API", nil, api},
}

// flags are the flags of a command, with --server where it talks to a
// Bugzilla.
type flags struct {
	*pflag.FlagSet
	server *string
}

func newFlags(use string, withServer bool) *flags {
	fs := pflag.NewFlagSet(use, pflag.ExitOnError)
	fs.SortFlags = false
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: bz %s [flags]\n\nFlags:\n%s", use, fs.FlagUsages())
	}
	f := &flags{FlagSet: fs}
	if withServer {
		f.server = fs.StringP("server", "R", "",
			"the Bugzilla: a name as samba, a host or a URL (default: bz server set-default)")
	}
	return f
}

// serverOf returns the Bugzilla of the flags.
func (f *flags) serverOf() (*server, error) {
	u, err := f.urlOf()
	if err != nil {
		return nil, err
	}
	return dial(u)
}

// urlOf returns the URL of the Bugzilla of the flags, and names it on
// standard error first, so that the user sees which Bugzilla a command
// acts on, and standard output keeps only what the command prints.
func (f *flags) urlOf() (string, error) {
	u, err := selectedURL(*f.server)
	if err == nil {
		u, err = bugzillaURL(u)
	}
	if err != nil {
		return "", err
	}
	fmt.Fprintln(os.Stderr, "Bugzilla:", u)
	return u, nil
}

// wantArgs checks that the arguments are between min and max in number, a
// max of -1 for any.
func (f *flags) wantArgs(min, max int) error {
	if n := f.NArg(); n < min || max >= 0 && n > max {
		f.Usage()
		return errors.New("wrong number of arguments")
	}
	return nil
}

func bugID(arg string) (int, error) {
	id, err := strconv.Atoi(arg)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("bad bug number %q", arg)
	}
	return id, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Search, read and file bugs in a Bugzilla.\n\nUsage: bz <group> <command> [flags]")
	for _, g := range groups {
		fmt.Fprintf(os.Stderr, "\n%s: %s\n", g.name, g.help)
		if g.run != nil {
			fmt.Fprintf(os.Stderr, "  %-24s %s\n", "bz "+g.name, g.help)
		}
		for _, c := range g.commands {
			fmt.Fprintf(os.Stderr, "  %-24s %s\n", "bz "+g.name+" "+c.name, c.help)
		}
	}
	fmt.Fprintln(os.Stderr, "\nRun bz <group> <command> --help for the flags of a command.")
}

func groupUsage(g group) {
	fmt.Fprintf(os.Stderr, "%s\n\nUsage: bz %s <command> [flags]\n\n", g.help, g.name)
	for _, c := range g.commands {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", c.name, c.help)
	}
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage()
		os.Exit(0)
	}
	for _, g := range groups {
		if g.name != args[0] {
			continue
		}
		if g.run != nil {
			if err := g.run(args[1:]); err != nil {
				fmt.Fprintln(os.Stderr, "bz:", err)
				os.Exit(1)
			}
			return
		}
		if len(args) > 1 {
			for _, c := range g.commands {
				if c.name == args[1] {
					if err := c.run(args[2:]); err != nil {
						fmt.Fprintln(os.Stderr, "bz:", err)
						os.Exit(1)
					}
					return
				}
			}
		}
		groupUsage(g)
		os.Exit(2)
	}
	usage()
	os.Exit(2)
}
