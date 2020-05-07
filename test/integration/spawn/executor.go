package run

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

type stream int

const (
	stdout stream = 1 << 0
	stderr stream = 1 << 1
	all    stream = stderr | stdout
)

func (s stream) String() string {
	return [...]string{"", "stdout", "stderr"}[s]
}

func (s stream) writer() io.Writer {
	if s == stderr {
		return os.Stderr
	}
	return os.Stdout
}

type line struct {
	kind stream
	data string
}

// Entry contains all the state of one command run.
type Entry struct {
	sync.Mutex

	path     string
	args     []string
	output   []line
	exitCode int
}

// Lines iterates over all lines of the command output.
func (e *Entry) Lines() chan<- string {
	ch := make(chan<- string)
	go func() {
		for _, line := range e.output {
			ch <- line.data
		}
	}()
	return ch
}

// ExitCode returns the exit status of the command.
func (e *Entry) ExitCode() int {
	return e.exitCode
}

// Contains return true if either stdout or stderr contains the string s.
func (e *Entry) Contains(s string) bool {
	for _, line := range e.output {
		if strings.Contains(line.data, s) {
			return true
		}
	}
	return false
}

// Executor holds the context for forking a number of commands.
type Executor struct {
	run *Run

	// showBreadcrumbs controls if Exectutor should diplay the command it is about to
	// run and its exit code.
	showBreadcrumbs bool
	// showOutput controls if Executor should forward the command output (stdout/err)
	// onto the its own stdout/err
	showOutput bool

	history []*Entry
}

func (e *Executor) handStream(entry *Entry, kind stream, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		entry.Lock()
		l := scanner.Text()
		entry.output = append(entry.output, line{
			kind: kind,
			data: l,
		})
		entry.Unlock()
		if e.showOutput {
			fmt.Fprintln(kind.writer(), l)
		}
	}
	return scanner.Err()
}

func exitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}

	if exiterr, ok := err.(*exec.ExitError); ok {
		// The program has exited with an exit code != 0

		// This works on both Unix and Windows. Although package
		// syscall is generally platform dependent, WaitStatus is
		// defined for both Unix and Windows and in both cases has
		// an ExitStatus() method with the same signature.
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), nil
		}
	}

	return -1, err
}

// RunV executes a command with an optional array of arguments. RunV will wait
// until the spawned commands terminates before returning.
func (e Executor) RunV(name string, args ...string) (*Entry, error) {
	return e.RunCmd(exec.Command(name, args...))
}

// RunCmd executes a exec.Cmd
func (e *Executor) RunCmd(cmd *exec.Cmd) (*Entry, error) {
	entry := &Entry{
		path: cmd.Path,
		args: cmd.Args,
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	syncChan := make(chan bool)
	go func() {
		e.handStream(entry, stdout, stdoutPipe)
		syncChan <- true
	}()

	go func() {
		e.handStream(entry, stderr, stderrPipe)
		syncChan <- true
	}()

	if e.showBreadcrumbs {
		fmt.Printf("=== EXE   %s\n", strings.Join(cmd.Args, " "))
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Make sure copying is finished
	<-syncChan
	<-syncChan

	err = cmd.Wait()
	exitCode, err := exitCode(err)
	entry.exitCode = exitCode

	if e.showBreadcrumbs {
		fmt.Printf("=== EXIT: %d\n", exitCode)
	}

	e.history = append(e.history, entry)

	return entry, err
}

// History return the nth entry. The most recent command is at index 0.
func (e *Executor) History(n int) *Entry {
	return e.history[len(e.history)-1-n]
}

// Last returns the most recent Entry.
func (e *Executor) Last() *Entry {
	return e.History(0)
}
