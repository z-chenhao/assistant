package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

func sha(data []byte) string { v := sha256.Sum256(data); return hex.EncodeToString(v[:]) }
func mac(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(value))
	return h.Sum(nil)
}

func signAWS(req *http.Request, body []byte, credentials AWSCredentials, now time.Time) error {
	if credentials.AccessKey == "" || credentials.SecretKey == "" || credentials.Region == "" {
		return &Error{Code: "AUTH_REQUIRED"}
	}
	date := now.UTC().Format("20060102")
	stamp := now.UTC().Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", stamp)
	req.Header.Set("X-Amz-Content-Sha256", sha(body))
	if credentials.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", credentials.SessionToken)
	}
	names := []string{"host"}
	headers := map[string]string{"host": req.URL.Host}
	for name, values := range req.Header {
		lower := strings.ToLower(name)
		if lower == "content-type" || strings.HasPrefix(lower, "x-amz-") {
			names = append(names, lower)
			headers[lower] = strings.Join(strings.Fields(strings.Join(values, ",")), " ")
		}
	}
	sort.Strings(names)
	var canonical strings.Builder
	for _, name := range names {
		canonical.WriteString(name + ":" + headers[name] + "\n")
	}
	query := strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20")
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	// Bedrock SigV4 canonicalizes the already escaped HTTP path. Inference
	// ARNs therefore retain their encoded label boundaries in the signature.
	var escaped strings.Builder
	const hexDigits = "0123456789ABCDEF"
	for _, b := range []byte(path) {
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("-._~/", rune(b)) {
			escaped.WriteByte(b)
		} else {
			escaped.WriteByte('%')
			escaped.WriteByte(hexDigits[b>>4])
			escaped.WriteByte(hexDigits[b&15])
		}
	}
	path = escaped.String()
	request := req.Method + "\n" + path + "\n" + query + "\n" + canonical.String() + "\n" + strings.Join(names, ";") + "\n" + sha(body)
	scope := date + "/" + credentials.Region + "/bedrock/aws4_request"
	key := mac([]byte("AWS4"+credentials.SecretKey), date)
	key = mac(key, credentials.Region)
	key = mac(key, "bedrock")
	key = mac(key, "aws4_request")
	signature := hex.EncodeToString(mac(key, "AWS4-HMAC-SHA256\n"+stamp+"\n"+scope+"\n"+sha([]byte(request))))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+credentials.AccessKey+"/"+scope+", SignedHeaders="+strings.Join(names, ";")+", Signature="+signature)
	return nil
}

func readEventStream(r io.Reader, handle func(string, []byte) error) error {
	for {
		prelude := make([]byte, 12)
		_, err := io.ReadFull(r, prelude)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		total := int(binary.BigEndian.Uint32(prelude))
		headersLength := int(binary.BigEndian.Uint32(prelude[4:]))
		if total < 16 || total > 16<<20 || headersLength > total-16 || crc32.ChecksumIEEE(prelude[:8]) != binary.BigEndian.Uint32(prelude[8:]) {
			return ErrInvalidResponse
		}
		message := make([]byte, total)
		copy(message, prelude)
		if _, err := io.ReadFull(r, message[12:]); err != nil {
			return err
		}
		if crc32.ChecksumIEEE(message[:total-4]) != binary.BigEndian.Uint32(message[total-4:]) {
			return ErrInvalidResponse
		}
		headers := message[12 : 12+headersLength]
		event := ""
		for len(headers) > 0 {
			n := int(headers[0])
			headers = headers[1:]
			if n+1 > len(headers) {
				return ErrInvalidResponse
			}
			name := string(headers[:n])
			kind := headers[n]
			headers = headers[n+1:]
			if kind != 7 || len(headers) < 2 {
				return ErrInvalidResponse
			}
			length := int(binary.BigEndian.Uint16(headers))
			headers = headers[2:]
			if length > len(headers) {
				return ErrInvalidResponse
			}
			value := string(headers[:length])
			headers = headers[length:]
			if name == ":event-type" || name == ":exception-type" {
				event = value
			}
		}
		if event == "" {
			return ErrInvalidResponse
		}
		if err := handle(event, message[12+headersLength:total-4]); err != nil {
			return err
		}
	}
}
