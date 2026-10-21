package memory

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/recall"
)

// Vault's key exists only in SecretStore. Ciphertext is bound to a record ID
// and scope; swapping records, changing ownership or key loss fails closed.
type Vault struct {
	Secrets llm.SecretStore
	Ref     string
}

func (v Vault) Digest(ctx context.Context, text string) (string, error) {
	if v.Secrets == nil || v.Ref == "" {
		return "", recall.ErrUnavailable
	}
	key, e := v.Secrets.Get(ctx, v.Ref)
	if e != nil || len(key) != 32 {
		return "", recall.ErrUnavailable
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte("memory-index-v1\x00" + text))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (v Vault) key(ctx context.Context) (cipher.AEAD, error) {
	if v.Secrets == nil || v.Ref == "" {
		return nil, recall.ErrUnavailable
	}
	k, e := v.Secrets.Get(ctx, v.Ref)
	if e != nil || len(k) != 32 {
		return nil, recall.ErrUnavailable
	}
	defer func() {
		for i := range k {
			k[i] = 0
		}
	}()
	b, e := aes.NewCipher(k)
	if e != nil {
		return nil, recall.ErrUnavailable
	}
	return cipher.NewGCM(b)
}
func (v Vault) Seal(ctx context.Context, plain, binding string) (string, error) {
	a, e := v.key(ctx)
	if e != nil {
		return "", e
	}
	n := make([]byte, a.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return "", recall.ErrUnavailable
	}
	return "sealed:v1:" + base64.RawStdEncoding.EncodeToString(a.Seal(n, n, []byte(plain), []byte(binding))), nil
}
func (v Vault) Open(ctx context.Context, sealed, binding string) (string, error) {
	const prefix = "sealed:v1:"
	if len(sealed) < len(prefix) || sealed[:len(prefix)] != prefix {
		return "", recall.ErrUnavailable
	}
	a, e := v.key(ctx)
	if e != nil {
		return "", e
	}
	raw, e := base64.RawStdEncoding.DecodeString(sealed[len(prefix):])
	if e != nil || len(raw) < a.NonceSize() {
		return "", recall.ErrUnavailable
	}
	p, e := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte(binding))
	if e != nil {
		return "", recall.ErrUnavailable
	}
	return string(p), nil
}
