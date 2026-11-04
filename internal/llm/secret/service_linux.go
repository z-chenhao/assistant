//go:build linux && cgo

package secret

/*
#cgo LDFLAGS: -ldl
#include <stdlib.h>
#include <string.h>
#include "service_linux.h"
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/z-chenhao/assistant/internal/llm"
)

const secretPrefix = "steward-secret-v1:"
const maxSecretBytes = 256 << 10
const maxEncodedSecret = len(secretPrefix) + (maxSecretBytes+2)/3*4

var serviceOnce sync.Once
var serviceAvailable bool

func serviceReady() bool {
	serviceOnce.Do(func() { serviceAvailable = C.steward_secret_init() != 0 })
	return serviceAvailable
}

// NativeBackend only checks the library. It never unlocks or reads a keyring.
func NativeBackend() (string, bool) { return "Linux Secret Service (libsecret)", serviceReady() }

func validServiceRef(ref string) bool {
	return ref != "" && len(ref) <= 512 && utf8.ValidString(ref) &&
		!strings.ContainsAny(ref, "\x00\r\n") && !strings.HasPrefix(ref, "env:")
}

// Cancellation uses libsecret's GCancellable, including while a desktop prompt
// is waiting. Join its watcher before releasing the native object.
func serviceContext(ctx context.Context) (context.Context, unsafe.Pointer, func(), error) {
	if ctx.Err() != nil || !serviceReady() {
		return nil, nil, nil, llm.ErrSecretStore
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	token := C.steward_secret_cancel_new()
	if token == nil {
		cancel()
		return nil, nil, nil, llm.ErrSecretStore
	}
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			C.steward_secret_cancel(token)
		case <-stop:
		}
	}()
	return ctx, token, func() {
		close(stop)
		<-stopped
		C.steward_secret_cancel_free(token)
		cancel()
	}, nil
}

func get(ctx context.Context, ref string) ([]byte, error) {
	if !validServiceRef(ref) {
		return nil, llm.ErrSecretStore
	}
	ctx, token, done, err := serviceContext(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	key := C.CString(ref)
	defer C.free(unsafe.Pointer(key))
	value := C.steward_secret_get(key, token)
	if value == nil {
		return nil, llm.ErrSecretStore
	}
	defer C.steward_secret_free(value)
	n := int(C.strnlen(value, C.size_t(maxEncodedSecret+1)))
	if ctx.Err() != nil || n > maxEncodedSecret {
		return nil, llm.ErrSecretStore
	}
	encoded := C.GoBytes(unsafe.Pointer(value), C.int(n))
	defer clear(encoded)
	if !bytes.HasPrefix(encoded, []byte(secretPrefix)) {
		return nil, llm.ErrSecretStore
	}
	out := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)-len(secretPrefix)))
	n, err = base64.StdEncoding.Strict().Decode(out, encoded[len(secretPrefix):])
	if err != nil || n == 0 || n > maxSecretBytes {
		clear(out)
		return nil, llm.ErrSecretStore
	}
	return out[:n], nil
}

func put(ctx context.Context, ref string, value []byte) error {
	if !validServiceRef(ref) || len(value) == 0 || len(value) > maxSecretBytes {
		return llm.ErrSecretStore
	}
	ctx, token, done, err := serviceContext(ctx)
	if err != nil {
		return err
	}
	defer done()
	key := C.CString(ref)
	defer C.free(unsafe.Pointer(key))
	encoded := make([]byte, len(secretPrefix)+base64.StdEncoding.EncodedLen(len(value))+1)
	copy(encoded, secretPrefix)
	base64.StdEncoding.Encode(encoded[len(secretPrefix):len(encoded)-1], value)
	defer clear(encoded)
	data := C.CBytes(encoded)
	defer func() {
		C.memset(data, 0, C.size_t(len(encoded)))
		C.free(data)
	}()
	if C.steward_secret_put(key, (*C.char)(data), token) == 0 || ctx.Err() != nil {
		return llm.ErrSecretStore
	}
	return nil
}

func remove(ctx context.Context, ref string) error {
	if !validServiceRef(ref) {
		return llm.ErrSecretStore
	}
	ctx, token, done, err := serviceContext(ctx)
	if err != nil {
		return err
	}
	defer done()
	key := C.CString(ref)
	defer C.free(unsafe.Pointer(key))
	if C.steward_secret_remove(key, token) == 0 || ctx.Err() != nil {
		return llm.ErrSecretStore
	}
	return nil
}
