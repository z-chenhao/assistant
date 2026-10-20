package fixture

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func browserPeerURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Path != "/call" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	return err == nil && port >= 1024 && port <= 65535
}

func (s *Server) browserCall(raw string) any {
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Post(s.browserEndpoint, "application/json", bytes.NewBufferString(raw))
	if err != nil {
		return map[string]string{"error": "UNKNOWN_SIDE_EFFECT"}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (12<<20)+1))
	var result map[string]any
	if err != nil || response.StatusCode != http.StatusOK || len(data) > 12<<20 || json.Unmarshal(data, &result) != nil {
		return map[string]string{"error": "UNKNOWN_SIDE_EFFECT"}
	}
	return result
}
