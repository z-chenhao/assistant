package feishu

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/z-chenhao/collie/internal/artifact"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

type secrets map[string][]byte

func (s secrets) Get(_ context.Context, k string) ([]byte, error) {
	return append([]byte{}, s[k]...), nil
}
func (secrets) Put(context.Context, string, []byte) error { return nil }
func (secrets) Delete(context.Context, string) error      { return nil }
func fixtureClient() Client {
	return Client{Config: Config{AppID: "app", TenantKey: "tenant", EncryptKeyRef: "key", VerificationTokenRef: "verify", AccessTokenRef: "access"}, Secrets: secrets{"key": []byte("encrypt-key"), "verify": []byte("verification-token"), "access": []byte("access-token")}}
}
func sign(raw []byte, now time.Time) http.Header {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	hash := sha256.Sum256(append([]byte(timestamp+"nonce"+"encrypt-key"), raw...))
	return http.Header{"X-Lark-Request-Timestamp": []string{timestamp}, "X-Lark-Request-Nonce": []string{"nonce"}, "X-Lark-Signature": []string{hex.EncodeToString(hash[:])}}
}
func TestSignedEncryptedCallbacksRejectForgeryWrongAppTenantTokenAndStale(t *testing.T) {
	c := fixtureClient()
	now := time.Now()
	raw := []byte(`{"schema":"2.0","header":{"token":"verification-token","app_id":"app","tenant_key":"tenant"}}`)
	if _, e := c.Verify(context.Background(), sign(raw, now), raw, now); e != nil {
		t.Fatal(e)
	}
	key := sha256.Sum256([]byte("encrypt-key"))
	block, _ := aes.NewCipher(key[:])
	pad := 16 - len(raw)%16
	data := append(append([]byte{}, raw...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	encrypted := make([]byte, 16+len(data))
	cipher.NewCBCEncrypter(block, encrypted[:16]).CryptBlocks(encrypted[16:], data)
	envelope, _ := json.Marshal(map[string]string{"encrypt": base64.StdEncoding.EncodeToString(encrypted)})
	got, e := c.Verify(context.Background(), sign(envelope, now), envelope, now)
	if e != nil || !bytes.Equal(got, raw) {
		t.Fatal("encrypted callback failed", e)
	}
	for _, bad := range [][]byte{bytes.ReplaceAll(raw, []byte(`"app"`), []byte(`"wrong"`)), bytes.ReplaceAll(raw, []byte(`"tenant"`), []byte(`"wrong"`)), bytes.ReplaceAll(raw, []byte("verification-token"), []byte("wrong-token")), []byte(`{"encrypt":"garbage"}`)} {
		if _, e = c.Verify(context.Background(), sign(bad, now), bad, now); e == nil {
			t.Fatal("invalid signed payload accepted")
		}
	}
	if _, e = c.Verify(context.Background(), sign(raw, now), append(raw, ' '), now); e == nil {
		t.Fatal("forgery accepted")
	}
	if _, e = c.Verify(context.Background(), sign(raw, now.Add(-6*time.Minute)), raw, now); e == nil {
		t.Fatal("stale accepted")
	}
}
func TestMessageAndAttachmentContractFixedDestinationAndNoRedirectCredentials(t *testing.T) {
	calls := 0
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token" {
			t.Error("missing scoped credential")
		}
		switch r.URL.Path {
		case "/open-apis/im/v1/files":
			if r.ParseMultipartForm(1<<20) != nil {
				t.Error("bad upload")
			}
			f, h, e := r.FormFile("file")
			if e != nil {
				t.Error(e)
			} else {
				defer f.Close()
				var b bytes.Buffer
				b.ReadFrom(f)
				if b.String() != "attachment" || h.Filename != "report.txt" {
					t.Error("wrong attachment")
				}
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"file_key":"file-key"}}`))
		case "/open-apis/im/v1/messages":
			calls++
			var q struct {
				ReceiveID string `json:"receive_id"`
				UUID      string `json:"uuid"`
				Content   string `json:"content"`
				Type      string `json:"msg_type"`
			}
			_ = json.NewDecoder(r.Body).Decode(&q)
			if q.ReceiveID != "bound-chat" || q.UUID == "" || q.Type != "file" || r.URL.Query().Get("receive_id_type") != "chat_id" || q.Content != `{"file_key":"file-key"}` {
				t.Error("wrong recipient/frozen action", q)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"remote-receipt"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer platform.Close()
	c := fixtureClient()
	c.Config.APIBase = platform.URL
	// The complete transport shape is asserted at the platform's actual HTTP edge.
	key, e := c.Upload(context.Background(), artifact.Descriptor{Name: "report.txt", ByteSize: 10}, bytes.NewBufferString("attachment"))
	if e != nil {
		t.Fatal(e)
	}
	id, e := c.Send(context.Background(), "bound-chat", "host-key", "file", map[string]string{"file_key": key})
	if e != nil || id != "remote-receipt" || calls != 1 {
		t.Fatal(id, e, calls)
	}
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	c.Config.APIBase = redirect.URL
	if _, e = c.Send(context.Background(), "bound-chat", "host-key", "text", map[string]string{"text": "safe"}); e == nil || leaked {
		t.Fatal("redirect leaked credentials")
	}
}

func TestURLVerificationUsesConfiguredTokenWithoutActivatingIngress(t *testing.T) {
	c := fixtureClient()
	raw := []byte(`{"type":"url_verification","token":"verification-token","challenge":"challenge"}`)
	if got, e := c.Verify(context.Background(), http.Header{}, raw, time.Now()); e != nil || !bytes.Equal(got, raw) {
		t.Fatal(e)
	}
	bad := bytes.ReplaceAll(raw, []byte("verification-token"), []byte("wrong-token"))
	if _, e := c.Verify(context.Background(), http.Header{}, bad, time.Now()); e == nil {
		t.Fatal("unverified challenge accepted")
	}
	ordinary := []byte(`{"header":{"token":"verification-token","app_id":"app","tenant_key":"tenant"}}`)
	if _, e := c.Verify(context.Background(), http.Header{}, ordinary, time.Now()); e == nil {
		t.Fatal("ordinary unsigned event accepted")
	}
}
