package sandbox

import (
	"bytes"
	"os/exec"
	"syscall"
	"time"
)

// Run owns process-group cancellation and joins transport cleanup. The caller
// supplies an already scoped command and chooses graceful or immediate stop.
// started distinguishes admission failure from a possible external effect;
// business error/retry policy belongs to the caller, not the process boundary.
func Run(cmd *exec.Cmd, stop syscall.Signal) (started bool, err error) {
	cmd.WaitDelay = time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, stop) }
	if err = cmd.Start(); err != nil {
		return false, err
	}
	defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	return true, cmd.Wait()
}

// Output captures a bounded subprocess stream. It deliberately does not embed
// bytes.Buffer: a promoted ReaderFrom would let io.Copy bypass the limit.
// Read only after the subprocess joins; concurrent writers are not supported.
type Output struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func NewOutput(limit int) *Output { return &Output{limit: max(0, limit)} }
func (o *Output) Bytes() []byte   { return o.buffer.Bytes() }
func (o *Output) Overflow() bool  { return o.overflow }
func (o *Output) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > o.limit-o.buffer.Len() {
		p = p[:o.limit-o.buffer.Len()]
		o.overflow = true
	}
	_, err := o.buffer.Write(p)
	return n, err
}
