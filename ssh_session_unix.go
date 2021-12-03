//go:build !windows
// +build !windows

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
	"fmt"
	"io"
	"log"
	"os/exec"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
)

func createPty(s ssh.Session, shell string) {
	var (
		ptyReq, winCh, _ = s.Pty()
		cmd              = exec.CommandContext(s.Context(), shell)
	)

	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
	f, err := pty.Start(cmd)
	if err != nil {
		log.Fatalln("Could not start shell:", err)
	}
	go func() {
		for win := range winCh {
			winSize := &pty.Winsize{Rows: uint16(win.Height), Cols: uint16(win.Width)}
			pty.Setsize(f, winSize)
		}
	}()

	go io.Copy(f, s)
	go io.Copy(s, f)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			log.Println("Session ended with error:", err)
		} else {
			log.Println("Session ended normally")
		}
		s.Exit(cmd.ProcessState.ExitCode())

	case <-s.Context().Done():
		log.Printf("Session terminated: %s", s.Context().Err())
		return
	}
}
