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
	"log"

	"github.com/gliderlabs/ssh"
)

// The following variables can be set via ldflags
var (
	localPassword = "letmeinbrudipls"
	authorizedKey = ""
	defaultShell  = "/bin/bash"
	version       = "1.2.0-dev"
)

func main() {
	p := setupParameters()

	var (
		forwardHandler = &ssh.ForwardedTCPHandler{}
		server         = ssh.Server{
			Handler: makeSSHSessionHandler(p.shell),
			PasswordHandler: ssh.PasswordHandler(func(ctx ssh.Context, pass string) bool {
				passed := pass == localPassword
				if passed {
					log.Printf("Successful authentication with password from %s@%s", ctx.User(), ctx.RemoteAddr().String())
				} else {
					log.Printf("Invalid password from %s@%s", ctx.User(), ctx.RemoteAddr().String())
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

	if authorizedKey != "" {
		server.PublicKeyHandler = ssh.PublicKeyHandler(func(ctx ssh.Context, key ssh.PublicKey) bool {
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
		})
	}

	run(p, server)
}
