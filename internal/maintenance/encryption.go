package maintenance

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"math"
)

// Each archive gets a random 256-bit salt and a derived AES key; each record
// has a strictly increasing 32-bit nonce suffix and a length in its AAD. The
// authenticated empty final record rejects truncation and appended content.
const magic = "ERII-BACKUP-1\n"
const chunk = 64 << 10

type encryptWriter struct {
	out    io.Writer
	a      cipher.AEAD
	prefix [32]byte
	seq    uint32
	closed bool
}

func archiveCipher(key []byte, salt [32]byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, ErrInvalid
	}
	h := hmac.New(sha256.New, key)
	h.Write([]byte(magic))
	h.Write(salt[:])
	derived := h.Sum(nil)
	defer func() {
		for i := range derived {
			derived[i] = 0
		}
	}()
	b, e := aes.NewCipher(derived)
	if e != nil {
		return nil, ErrInvalid
	}
	return cipher.NewGCM(b)
}
func encrypt(out io.Writer, key []byte) (*encryptWriter, error) {
	w := &encryptWriter{out: out}
	if _, e := rand.Read(w.prefix[:]); e != nil {
		return nil, e
	}
	a, e := archiveCipher(key, w.prefix)
	if e != nil {
		return nil, e
	}
	w.a = a
	_, e = out.Write(append([]byte(magic), w.prefix[:]...))
	return w, e
}
func nonce(prefix [32]byte, seq uint32) []byte {
	v := make([]byte, 12)
	copy(v, prefix[:8])
	binary.BigEndian.PutUint32(v[8:], seq)
	return v
}
func aad(n []byte, size uint32) []byte {
	v := append([]byte(magic), n...)
	return binary.BigEndian.AppendUint32(v, size)
}
func (w *encryptWriter) record(p []byte) error {
	if w.closed || w.seq == math.MaxUint32 {
		return ErrInvalid
	}
	n := nonce(w.prefix, w.seq)
	w.seq++
	if _, e := w.out.Write(binary.BigEndian.AppendUint32(nil, uint32(len(p)))); e != nil {
		return e
	}
	_, e := w.out.Write(w.a.Seal(nil, n, p, aad(n, uint32(len(p)))))
	return e
}
func (w *encryptWriter) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		n := min(len(p), chunk)
		if e := w.record(p[:n]); e != nil {
			return total, e
		}
		total += n
		p = p[n:]
	}
	return total, nil
}
func (w *encryptWriter) Close() error { e := w.record(nil); w.closed = true; return e }

type decryptReader struct {
	in      io.Reader
	a       cipher.AEAD
	prefix  [32]byte
	seq     uint32
	pending []byte
	final   bool
}

func decrypt(in io.Reader, key []byte) (*decryptReader, error) {
	if len(key) != 32 {
		return nil, ErrInvalid
	}
	header := make([]byte, len(magic)+32)
	if _, e := io.ReadFull(in, header); e != nil || string(header[:len(magic)]) != magic {
		return nil, ErrInvalid
	}
	r := &decryptReader{in: in}
	copy(r.prefix[:], header[len(magic):])
	a, e := archiveCipher(key, r.prefix)
	if e != nil {
		return nil, e
	}
	r.a = a
	return r, nil
}
func (r *decryptReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	if r.final {
		return 0, io.EOF
	}
	if r.seq == math.MaxUint32 {
		return 0, ErrInvalid
	}
	var size uint32
	if binary.Read(r.in, binary.BigEndian, &size) != nil || size > chunk {
		return 0, ErrInvalid
	}
	sealed := make([]byte, int(size)+r.a.Overhead())
	if _, e := io.ReadFull(r.in, sealed); e != nil {
		return 0, ErrInvalid
	}
	n := nonce(r.prefix, r.seq)
	r.seq++
	plain, e := r.a.Open(nil, n, sealed, aad(n, size))
	if e != nil {
		return 0, ErrInvalid
	}
	if size == 0 {
		var extra [1]byte
		n, e := io.ReadFull(r.in, extra[:])
		if n != 0 || e != io.EOF {
			return 0, ErrInvalid
		}
		r.final = true
		return 0, io.EOF
	}
	r.pending = plain
	return r.Read(p)
}
