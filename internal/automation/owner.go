package automation

import (
	"strings"
	"time"
	"unicode/utf8"
)

// OwnerConfiguration is the private owner editor command, separate from the
// model-visible management tool. Disabling a schedule never cancels its Task.
type OwnerConfiguration struct {
	Ref, Instruction, Type, Spec, Timezone string
	// PreserveSpec authenticates an unchanged exact editor instant on a new edit.
	PreserveSpec string
	Version      int64
	Enabled      bool
	Below        *int64
}
type OwnerReceipt struct {
	Ref     string
	Version int64
}

func (q OwnerConfiguration) Validate() error {
	if q.PreserveSpec != "" && (q.Ref == "" || q.Type != "at" || q.PreserveSpec != q.Spec) {
		return ErrInvalid
	}
	if (q.Ref == "" && q.Version != 0) || (q.Ref != "" && (!strings.HasPrefix(q.Ref, "automation:") || len(q.Ref) > 256 || strings.ContainsAny(q.Ref, "/\\\x00\r\n") || q.Version < 1)) {
		return ErrInvalid
	}
	if strings.TrimSpace(q.Instruction) == "" || !utf8.ValidString(q.Instruction) || len(q.Instruction) > 8192 || len(q.Spec) > 256 || len(q.Timezone) > 128 || (q.Below != nil && (*q.Below < 0 || *q.Below > 1000000000000)) {
		return ErrInvalid
	}
	_, err := Next(q.Type, q.Spec, q.Timezone, time.Now())
	return err
}
