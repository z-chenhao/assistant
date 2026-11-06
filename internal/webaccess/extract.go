package webaccess

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

func extractDocument(ctx context.Context, c Config, source, kind string, data []byte) (string, string, error) {
	if c.ExtractScript == "" {
		if kind == "text/html" {
			title, text := extractHTML(string(data))
			return title, text, nil
		}
		return "", "", errors.New("WEB_EXTRACTOR_UNAVAILABLE")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	root, e := os.MkdirTemp("", "steward-web-extract-")
	if e != nil {
		return "", "", e
	}
	defer os.RemoveAll(root)
	raw, _ := json.Marshal(struct {
		URL       string `json:"url"`
		MediaType string `json:"mediaType"`
		Data      []byte `json:"data"`
	}{source, kind, data})
	command := exec.CommandContext(ctx, c.ExtractCommand, c.ExtractScript)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	command.WaitDelay = time.Second
	command.Dir = root
	command.Env = []string{"HOME=" + root, "TMPDIR=" + root, "PATH=" + filepath.Dir(c.ExtractCommand) + ":/usr/bin:/bin", "LANG=C.UTF-8"}
	command.Stdin = bytes.NewReader(raw)
	command.Stderr = io.Discard
	stdout, e := command.StdoutPipe()
	if e != nil {
		return "", "", e
	}
	if command.Start() != nil {
		return "", "", errors.New("WEB_EXTRACTOR_UNAVAILABLE")
	}
	defer func() { _ = command.Cancel(); _ = command.Wait() }()
	result, e := io.ReadAll(io.LimitReader(stdout, (8<<20)+1))
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	if e != nil || len(result) > 8<<20 {
		return "", "", errors.New("RESOURCE_LIMIT")
	}
	var response struct {
		Title string `json:"title"`
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if json.Unmarshal(result, &response) != nil || response.Error != "" || response.Text == "" {
		return "", "", errors.New("WEB_EXTRACTION_FAILED")
	}
	return response.Title, response.Text, nil
}
func FetchContent(ctx context.Context, c Config, raw, mode string) (Page, error) {
	return fetchContent(ctx, c, raw, mode, publicClient())
}
func fetchContent(ctx context.Context, c Config, raw, mode string, client interface {
	Do(*http.Request) (*http.Response, error)
}) (Page, error) {
	page := Page{URL: raw, Untrusted: true, Freshness: "fetched_now; origin may serve cached content"}
	if ValidURL(raw) != nil {
		return page, errBlocked
	}
	if mode != "readable" && mode != "raw" && mode != "answer" && mode != "" {
		return page, errors.New("INVALID_ARGUMENT")
	}
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return page, errBlocked
	}
	request.Header.Set("User-Agent", "Steward-WebAccess/2.0")
	request.Header.Set("Accept", "text/html,text/markdown,text/plain,application/pdf,image/png,image/jpeg")
	response, e := client.Do(request)
	if e != nil {
		if ctx.Err() != nil {
			return page, ctx.Err()
		}
		return page, errors.New("WEB_FETCH_FAILED")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return page, errors.New("WEB_HTTP_OR_ACCESS_ERROR")
	}
	kind, _, e := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if e != nil {
		return page, errors.New("WEB_UNSUPPORTED_CONTENT")
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if e != nil || len(data) > 8<<20 {
		return page, errors.New("RESOURCE_LIMIT")
	}
	page.URL = response.Request.URL.String()
	page.MediaType = kind
	page.RetrievedAt = time.Now().UTC().Format(time.RFC3339)
	if strings.HasPrefix(kind, "image/") {
		if mode == "raw" || mode == "answer" {
			return page, errors.New("WEB_UNSUPPORTED_CONTENT")
		}
		image, e := NormalizeImage(base64.StdEncoding.EncodeToString(data), kind)
		if e != nil {
			return page, e
		}
		page.Image = image
		page.Text = fmt.Sprintf("Image fetched (%d x %d)", image.Width, image.Height)
		return page, nil
	}
	switch kind {
	case "text/html", "application/pdf":
		if kind == "text/html" && !utf8.Valid(data) {
			return page, errors.New("WEB_UNSUPPORTED_ENCODING")
		}
		if mode == "raw" {
			if kind != "text/html" {
				return page, errors.New("WEB_UNSUPPORTED_CONTENT")
			}
			if !utf8.Valid(data) {
				return page, errors.New("WEB_UNSUPPORTED_ENCODING")
			}
			page.Text = string(data)
		} else {
			page.Title, page.Text, e = extractDocument(ctx, c, page.URL, kind, data)
			if e != nil {
				return page, e
			}
		}
	case "text/plain", "text/markdown":
		if !utf8.Valid(data) {
			return page, errors.New("WEB_UNSUPPORTED_ENCODING")
		}
		page.Text = string(data)
	default:
		return page, errors.New("WEB_UNSUPPORTED_CONTENT")
	}
	if strings.TrimSpace(page.Text) == "" {
		return page, errors.New("WEB_EMPTY_OR_DYNAMIC_PAGE")
	}
	return page, nil
}
