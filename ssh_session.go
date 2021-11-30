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
	"io"
	"log"
	"os/exec"

	"github.com/gliderlabs/ssh"
)

func createSSHSessionHandler(shell string) ssh.Handler {
	return func(s ssh.Session) {
		log.Printf("New login from %s@%s", s.User(), s.RemoteAddr().String())
		_, _, isPty := s.Pty()

		switch {
		case isPty:
			log.Println("PTY requested")

			createPty(s, shell)

		case len(s.Command()) > 0:
			log.Printf("No PTY requested, executing command: '%s'", s.RawCommand())

			cmd := exec.Command(s.Command()[0], s.Command()[1:]...)
			// We use StdinPipe to avoid blocking on missing input
			if stdIn, err := cmd.StdinPipe(); err != nil {
				log.Println("Could not initialize StdInPipe", err)
				s.Exit(1)
				return
			} else {
				go func() {
					if _, err := io.Copy(stdIn, s); err != nil {
						log.Printf("Error while copying input from %s to stdIn: %s", s.RemoteAddr().String(), err)
					}
					if err := stdIn.Close(); err != nil {
						log.Println("Error while closing stdInPipe:", err)
					}
				}()
			}
			cmd.Stdout = s
			cmd.Stderr = s

			done := make(chan error, 1)
			go func() { done <- cmd.Run() }()

			select {
			case err := <-done:
				if err != nil {
					log.Println("Command execution failed:", err)
					io.WriteString(s, "Command execution failed: "+err.Error())
				} else {
					log.Println("Command execution successful")
				}
				s.Exit(cmd.ProcessState.ExitCode())

			case <-s.Context().Done():
				log.Println("Session closed by remote, killing dangling process")
				if cmd.Process != nil && cmd.ProcessState == nil {
					if err := cmd.Process.Kill(); err != nil {
						log.Println("Failed to kill process:", err)
					}
				}
			}

		default:
			log.Println("No PTY requested, no command supplied")

			select {
			case <-s.Context().Done():
				log.Println("Session closed")
			}
		}
	}
}
