// +build windows

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
	"os"
	"os/exec"
	"syscall"

	"github.com/ActiveState/termtest/conpty"
	"github.com/gliderlabs/ssh"
	"golang.org/x/sys/windows"
)

func createPty(s ssh.Session, shell string) {
	ptyReq, winCh, _ := s.Pty()
	vsn := windows.RtlGetVersion()

	if vsn.MajorVersion < 10 ||
		vsn.BuildNumber < 17763 {
		// Interactive Pty via ssh-shellhost.exe

		log.Println("Windows version too old to support ConPTY shell")

		if shell == defaultShell {
			log.Println("No fully interactive shell available, denying PTY request")
			io.WriteString(s, "No ConPTY shell or ssh-shellhost enhanced shell available. "+
				"Please append 'cmd' to your ssh command to gain shell access, i.e. "+
				"'ssh <OPTIONS> <IP> cmd'.\n")
			s.Exit(1)
			return
		}
		log.Println("Launching shell with ssh-shellhost.exe")

		cmd := exec.Command(shell)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CmdLine:       " " + "---pty cmd", // Must leave a space to the beginning
			CreationFlags: 0x08000000,
		}
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
				log.Println("Session ended with error:", err)
			} else {
				log.Println("Session ended normally")
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

	} else {
		// Interactive ConPTY

		cpty, err := conpty.New(int16(ptyReq.Window.Width), int16(ptyReq.Window.Height))
		if err != nil {
			log.Fatalf("Could not open a conpty terminal: %v", err)
		}
		defer cpty.Close()

		// Dynamically handle resizes of terminal window
		go func() {
			for win := range winCh {
				cpty.Resize(uint16(win.Width), uint16(win.Height))
			}
		}()

		// Spawn and catch new powershell process
		pid, _, err := cpty.Spawn(
			"C:\\WINDOWS\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
			[]string{},
			&syscall.ProcAttr{
				Env: os.Environ(),
			},
		)
		if err != nil {
			log.Fatalf("Could not spawn a powershell: %v", err)
		}
		log.Printf("New process with pid %d spawned", pid)
		process, err := os.FindProcess(pid)
		if err != nil {
			log.Fatalf("Failed to find process: %v", err)
		}

		// Link data streams of ssh session and conpty
		go io.Copy(s, cpty.OutPipe())
		go io.Copy(cpty.InPipe(), s)

		done := make(chan struct {
			*os.ProcessState
			error
		}, 1)
		go func() {
			ps, err := process.Wait()
			done <- struct {
				*os.ProcessState
				error
			}{ps, err}
		}()

		select {
		case result := <-done:
			if result.error != nil {
				log.Println("Error waiting for process:", err)
				s.Exit(1)
				return
			}
			log.Printf("Session ended normally, exit code %d", result.ProcessState.ExitCode())
			s.Exit(result.ProcessState.ExitCode())

		case <-s.Context().Done():
			log.Println("Session closed by remote, killing process")
			if err := process.Kill(); err != nil {
				log.Println("Failed to kill process:", err)
			}
		}
	}
}
