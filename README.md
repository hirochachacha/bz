# bz

`bz` searches, reads and files bugs in a Bugzilla through its REST API,
with commands in the manner of [gh](https://cli.github.com/):
`bz bug list`, `bz bug view`, `bz auth login`.

## Install

It needs Go 1.27 or later.

```sh
go install github.com/hirochachacha/bz@latest
```

## Choosing the Bugzilla

A command uses the Bugzilla of `-R`/`--server`, else the default that
`bz server set-default` set. Each takes a URL, a host, or the name of a
well-known one: apache, freebsd, gcc, gentoo, kde, kernel, libreoffice,
mozilla, openssh, opensuse, redhat, samba, sourceware, suse, webkit.

## Reading

Reading needs no API key, on most Bugzillas.

```sh
bz bug list -R samba -S 'notifyd inotify'        # search by words
bz bug view -R samba 16303 --comments            # fields, attachments, comments
bz product view -R samba 'Samba 4.1 and newer'   # components and versions
bz attachment download -R samba 19329            # save an attachment
bz api -R samba bug/16303 -f include_fields=id,status
```

## Filing

Changing a bug needs an API key. `bz auth login` opens the page of the
Bugzilla that makes one, reads the key from the terminal and stores it in
`~/.config/bz/<host>`.

```sh
bz auth login -R samba
bz bug create -R samba --dry-run \
  -t 'smbd crashes on ...' --product 'Samba 4.1 and newer' \
  --component 'File services' --version 4.24.6 \
  -F description.txt --attach repro.sh --attach fix.patch
bz bug comment -R samba 16303 -F reply.txt
bz bug attach -R samba 16303 fix-v2.patch --obsoletes 19329
bz bug close -R samba 16303 --duplicate-of 16200
```

`bug create --dry-run` checks the fields against the Bugzilla and prints
the bug without sending it. The commands that change a bug show what they
send and ask first on a terminal; elsewhere they send only with `--yes`.

Run `bz help` for all the commands, and `bz <group> <command> --help` for
the flags of one.

## Skill for agents

`bz` carries a skill that tells a coding agent how to use it.
`bz skill install` installs it into `./.agents/skills/bz`, and
`bz skill show` prints it.

## License

[MIT](LICENSE)
