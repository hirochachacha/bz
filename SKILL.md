---
name: bugzilla
description: >-
  Search, read and file bugs in a Bugzilla (bugzilla.samba.org,
  bugzilla.redhat.com, bugzilla.kernel.org and others) with the bz CLI:
  look for existing bugs, follow a bug's comments, file a bug with its
  reproducer and patch, add comments and attachments. Use when a bug of
  other software is to be filed upstream in a Bugzilla, checked for
  duplicates or followed up.
allowed-tools: Bash(bz:*)
---

# Bugzilla

`bz` talks to the REST API of a Bugzilla, at `/rest` or, where the web
server does not rewrite it, `/rest.cgi`; it finds which. Run `bz help`,
and `bz <command> -h`, for the commands and their flags.

## Choosing the Bugzilla

A command uses the Bugzilla of `-server` (before the command), else the
default that `bz use` set; `bz use` alone prints it, and the first
`login` sets its Bugzilla. Each takes a URL, a host, or the name of
a well-known one: apache, freebsd, gcc, gentoo, kde, kernel, libreoffice,
mozilla, openssh, opensuse, redhat, samba, sourceware, suse, webkit. Pass
`-server` rather than changing the default, which other sessions share.
Some, as apache, ask for a login even to search.

## Reading needs no key

```sh
bz -server samba search notifyd inotify          # words alone: quicksearch
bz -server samba show 16303                      # fields, attachments, comments
bz -server samba status                          # the user's open bugs (needs the key)
bz -server samba search -reporter @me -open      # fields, with or without words
bz -server samba product                         # products that take new bugs
bz -server samba product 'Samba 4.1 and newer'   # components and versions
bz -server samba field severity                  # the values a field takes
```

`status` lists the open bugs that the user reported, is assigned or is
CC'd to, the last changed first. `search` takes words, fields
(`-reporter`, `@me` for the user; `-product`, `-component`, `-open`), or
both; with fields the words match the summary, as quicksearch ignores
other criteria.

`product` and `field` list only the active choices, those a new bug can
take; a Bugzilla keeps old versions and the like inactive. The choices are
cached for a day in `~/.cache/bz` (under `$XDG_CACHE_HOME` if set);
`bz cache clear` drops the cache of the Bugzilla in use. Some Bugzillas cannot
list the components of a large product (Red Hat's Fedora gives up after
minutes); then `product` lists the versions only, and `create -dry-run`
says that it did not check the component.

## Filing

Before filing, `search` for the bug, by its summary and by the words of
its symptom; show the user what may be the same bug.

```sh
bz -server samba create -dry-run \
  -product 'Samba 4.1 and newer' -component 'File services' \
  -version 4.24.6 -summary '...' -body-file description.txt \
  -set op_sys=Linux -attach repro.sh -attach fix.patch
```

`create` takes the product, component, version and summary, the
description from `-body` or `-body-file` (`-` for standard input), any
other field of creating a bug with `-set name=value` (as `op_sys`,
`platform`, `severity`, `priority`, `keywords`, `cc`; a name repeated
makes a list), and files to attach with `-attach`, `.patch` and `.diff` as
patches, each under its own name. With `-dry-run` it checks the product,
component, version and the fields whose values the Bugzilla lists, and
prints the bug without sending it. A version must be one of the product;
where the bug is of one not listed (as `master`), take `unspecified` and
say the version in the description.

A description file that begins with a header of fields (`Product: ...`)
and a blank line gives the flags; pass the rest as the description, as
`tail -n +N file | bz create ... -body-file -`.

## Sending needs the user's approval each time

`create`, `comment`, `attach` and `close` publish to a public tracker. On a
terminal they show what they send and ask; elsewhere they send only with
`-yes`. Pass `-yes` only after the user approved that bug or text in this
conversation; show them the output of `create -dry-run`, or the text,
first. Reports for a security team are not filed in a public Bugzilla; the
user reports them by hand.

```sh
bz -server samba create -yes ...
bz -server samba comment -yes -body-file reply.txt 16303
bz -server samba attach -yes [-comment TEXT] [-obsoletes ID] 16303 fix-v2.patch
bz -server samba close -yes -resolution INVALID [-body TEXT] 16303
```

A revised patch goes up with `attach -obsoletes <attachment id>` of the
patch it replaces; `show` lists the ids, and `attach` refuses an id of
another bug. Bugzilla deletes nothing for a user: `close` resolves a bug
(`-resolution DUPLICATE -dupe-of <bug>` for a duplicate; `-status CLOSED`
where the Bugzilla closes so), and obsoleting hides an attachment.

`create` prints the bug's URL. If an attachment fails, the bug exists:
attach the rest with `attach`; do not create it again.

They need the user's API key in `~/.config/bz/<host>` (under
`$XDG_CONFIG_HOME` if set), as `~/.config/bz/bugzilla.samba.org`. Without
it, ask the user to run `bz login <Bugzilla>` (as `bz login samba`) in a
terminal of their own: it asks for their login, shows where to make a key,
reads the key, checks it and stores it (mode 600). Do not run
`login` yourself, and never print, copy or ask for the key in the chat.

## After filing

Record the bug's URL where the project tracks the report, and update its
status there, as its instructions ask. Follow up with `show` when the user
asks about the state of a bug.
