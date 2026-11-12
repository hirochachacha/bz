---
name: bugzilla
description: >-
  Search, read and file bugs in a Bugzilla (bugzilla.samba.org,
  bugzilla.redhat.com, bugzilla.kernel.org and others) with the bz CLI:
  look for existing bugs, follow a bug's comments, file a bug with its
  reproducer and patch, add comments and attachments, close a bug. Use when
  a bug of other software is to be filed upstream in a Bugzilla, checked
  for duplicates or followed up.
allowed-tools: Bash(bz:*)
---

# Bugzilla

`bz` talks to the REST API of a Bugzilla, at `/rest` or, where the web
server does not rewrite it, `/rest.cgi`; it finds which. Its commands
follow gh: `bz <group> <command>`, flags anywhere, `--help` on each. Run
`bz help` for them all.

## Choosing the Bugzilla

A command uses the Bugzilla of `-R`/`--server`, else the default that
`bz server set-default` set (`--view` prints it); the first
`auth login` sets its Bugzilla as the default. Each takes a URL, a host,
or the name of a well-known one: apache, freebsd, gcc, gentoo, kde,
kernel, libreoffice, mozilla, openssh, opensuse, redhat, samba,
sourceware, suse, webkit. Pass `-R` rather than changing the default,
which other sessions share. Each command names its Bugzilla first, on
standard error. Some, as apache, ask for a login even to search.

## Reading needs no key

```sh
bz bug list -R samba -S 'notifyd inotify'        # words: quicksearch
bz bug list -R samba -A @me -s all               # fields; @me needs the key
bz bug status -R samba                           # your open bugs (needs the key)
bz bug view -R samba 16303 [--comments]          # fields, attachments, comments
bz product list -R samba                         # products that take new bugs
bz product view -R samba 'Samba 4.1 and newer'   # components and versions
bz field view -R samba severity                  # the values a field takes
bz attachment download -R samba 19329 [-O file]  # save under its name; --clobber
bz api -R samba bug/16303 -f include_fields=id,status   # any GET of the REST API
```

`bug list` takes `-A`/`--author` (the reporter), `-a`/`--assignee`
(`@me` for the user), `--product`, `--component`, `-s`/`--state`
(`open`, the default, `closed` or `all`), `-S`/`--search` and
`-L`/`--limit`. Words alone go to quicksearch, which searches the
comments too; with fields they match the summary, as quicksearch ignores
other criteria. `bug status` lists the open bugs that the user reported,
is assigned or is CC'd to, the last changed first.

`product` and `field` list only the active choices, those a new bug can
take; a Bugzilla keeps old versions and the like inactive. The choices
are cached for a day in `~/.cache/bz` (under `$XDG_CACHE_HOME` if set);
`bz cache clear` drops the cache of a Bugzilla. Some Bugzillas cannot
list the components of a large product (Red Hat's Fedora gives up after
minutes); then `product view` lists the versions only, `bug create
--dry-run` says that it did not check the component, and the timeout is
cached too.

## Filing

Before filing, `bug list -S` for the bug, by its summary and by the words
of its symptom, with `-s all`; show the user what may be the same bug.

```sh
bz bug create -R samba --dry-run \
  -t '...' --product 'Samba 4.1 and newer' --component 'File services' \
  --version 4.24.6 -F description.txt \
  --set op_sys=Linux --attach repro.sh --attach fix.patch
```

`bug create` takes the summary (`-t`/`--title`), product, component and
version, the description from `-b`/`--body` or `-F`/`--body-file` (`-`
for standard input), any other field of creating a bug with `--set
name=value` (as `op_sys`, `platform`, `severity`, `priority`, `keywords`,
`cc`; a name repeated makes a list), and files to attach with `--attach`,
`.patch` and `.diff` as patches, each under its own name. With
`--dry-run` it checks the product, component, version and the fields
whose values the Bugzilla lists, and prints the bug without sending it. A
version must be one of the product; where the bug is of one not listed
(as `master`), take `unspecified` and say the version in the description.

A description file that begins with a header of fields (`Product: ...`)
and a blank line gives the flags; pass the rest as the description, as
`tail -n +N file | bz bug create ... -F -`.

## Sending needs the user's approval each time

`bug create`, `comment`, `attach`, `edit`, `close` and `reopen`, and `api`
with a method other than GET, publish to a public tracker. On a terminal
they show what they send and ask; elsewhere they send only with `--yes`.
Pass `--yes` only after the user approved that bug or text in this
conversation; show them the output of `bug create --dry-run`, or the text,
first. Reports for a security team are not filed in a public Bugzilla; the
user reports them by hand.

```sh
bz bug create -R samba --yes ...
bz bug comment -R samba 16303 -F reply.txt --yes
bz bug attach -R samba 16303 fix-v2.patch --obsoletes 19329 [-c TEXT] --yes
bz bug close -R samba 16303 -r INVALID [-c TEXT] --yes
bz bug close -R samba 16303 --duplicate-of 16200 --yes
bz bug reopen -R samba 16303 [--status UNCONFIRMED] [-c TEXT] --yes
bz bug edit -R samba 16303 -t '...' --version 4.24.6 --add-cc LOGIN [-c TEXT] --yes
bz api -R samba -X PUT bug/16303 -f keywords=patch --yes
```

`bug create` prints the bug's URL. If an attachment fails, the bug exists:
attach the rest with `bug attach`; do not create it again. A revised patch
goes up with `--obsoletes <attachment id>` of the patch it replaces; `bug
view` lists the ids, and `bug attach` refuses an id of another bug.
Bugzilla deletes nothing for a user: `bug close` resolves a bug (`--status
CLOSED` where the Bugzilla closes so), and obsoleting hides an attachment.
`bug reopen` takes REOPENED, else CONFIRMED; where the workflow or the
user's permissions refuse it (a reporter in Mozilla's Bugzilla), pass
`--status UNCONFIRMED`. `bug edit` changes the summary (`-t`), product,
component and version, the CC list and keywords (`--add-cc`,
`--remove-cc`, `--add-keyword`, `--remove-keyword`) and any other field
(`--set`), checked as `bug create` checks them; a Bugzilla may refuse a
reporter some fields, as Mozilla's the severity. `api` does what the
commands do not, as `gh api` does: fields go with `-f key=value` (strings)
or `-F key=value` (JSON literals, numbers, `@file`), in the query of a GET
and the JSON body of other methods, or the body with `--input`.

They need the user's API key in `~/.config/bz/<host>` (under
`$XDG_CONFIG_HOME` if set), as `~/.config/bz/bugzilla.samba.org`. `bz auth
status` says which Bugzillas have a key that logs in. Without one, ask the
user to run `bz auth login -R <Bugzilla>` (as `bz auth login -R samba`) in
a terminal of their own: it opens the page to make a key in their browser
where it can, else shows its URL, reads the key, checks it and stores it
(mode 600). Do not run `auth login` yourself, and never print, copy or ask
for the key in the chat.

## After filing

Record the bug's URL where the project tracks the report, and update its
status there, as its instructions ask. Follow up with `bug view
--comments` or `bug status` when the user asks about the state of a bug.
