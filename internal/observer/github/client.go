package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const maxPages = 20
const pageSize = 100

var shaPattern = regexp.MustCompile(`^[a-fA-F0-9]{40,64}$`)

type reader struct {
	client http.Client
	token  string
}

func (c Client) read(ctx context.Context, cfg Config) (Batch, error) {
	var batch Batch
	if c.Secrets == nil {
		return batch, &Failure{"CREDENTIAL_MISSING"}
	}
	secret, err := c.Secrets.Get(ctx, cfg.CredentialRef)
	if err != nil || len(secret) == 0 || len(secret) > 16<<10 {
		return batch, &Failure{"CREDENTIAL_MISSING"}
	}
	r := reader{client: http.Client{Timeout: 20 * time.Second}, token: string(secret)}
	for i := range secret {
		secret[i] = 0
	}
	if c.HTTP != nil {
		r.client = *c.HTTP
	}
	r.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err = r.get(ctx, "/user", &user); err != nil {
		return batch, err
	}
	if user.ID <= 0 || user.Login == "" {
		return batch, &Failure{"INVALID_RESPONSE"}
	}
	path := "/repos/" + cfg.Repository
	var pr struct {
		Number int64 `json:"number"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Repo struct {
				ID       int64  `json:"id"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
	}
	if err = r.get(ctx, path+"/pulls/"+fmt.Sprint(cfg.PullNumber), &pr); err != nil {
		return batch, err
	}
	if pr.Number != cfg.PullNumber || pr.Base.Repo.ID <= 0 || !strings.EqualFold(pr.Base.Repo.FullName, cfg.Repository) || !shaPattern.MatchString(pr.Head.SHA) {
		return batch, &Failure{"INVALID_RESPONSE"}
	}
	batch.RepositoryID, batch.HeadSHA = pr.Base.Repo.ID, pr.Head.SHA
	for _, kind := range []string{"review_comment", "review"} {
		endpoint := path + "/pulls/" + fmt.Sprint(cfg.PullNumber) + "/comments"
		if kind == "review" {
			endpoint = path + "/pulls/" + fmt.Sprint(cfg.PullNumber) + "/reviews"
		}
		for page := 1; ; page++ {
			var entries []struct {
				ID        int64     `json:"id"`
				Body      string    `json:"body"`
				State     string    `json:"state"`
				URL       string    `json:"html_url"`
				Updated   time.Time `json:"updated_at"`
				Submitted time.Time `json:"submitted_at"`
				User      struct {
					ID    int64  `json:"id"`
					Login string `json:"login"`
				} `json:"user"`
			}
			if err = r.get(ctx, endpoint+fmt.Sprintf("?per_page=%d&page=%d", pageSize, page), &entries); err != nil {
				return Batch{}, err
			}
			if entries == nil {
				return Batch{}, &Failure{"INVALID_RESPONSE"}
			}
			for _, entry := range entries {
				if entry.ID <= 0 || entry.User.ID <= 0 || entry.User.Login == "" || !sourceURL(entry.URL, cfg) {
					return Batch{}, &Failure{"INVALID_RESPONSE"}
				}
				if entry.User.ID == user.ID || kind == "review" && (entry.State == "PENDING" || entry.Submitted.IsZero()) {
					continue
				}
				at := entry.Updated
				if at.IsZero() {
					at = entry.Submitted
				}
				if at.IsZero() {
					return Batch{}, &Failure{"INVALID_RESPONSE"}
				}
				e := Observation{Kind: kind, SourceID: entry.ID, URL: entry.URL, Actor: entry.User.Login, Body: boundedText(entry.Body), State: entry.State, UpdatedAt: at.UTC()}
				e.identify()
				if err = batch.add(e); err != nil {
					return Batch{}, err
				}
			}
			if len(entries) < pageSize {
				break
			}
			if page == maxPages {
				return Batch{}, &Failure{"HISTORY_LIMIT"}
			}
		}
	}
	for page := 1; ; page++ {
		var checks struct {
			Runs []struct {
				ID         int64     `json:"id"`
				Name       string    `json:"name"`
				Status     string    `json:"status"`
				Conclusion string    `json:"conclusion"`
				HeadSHA    string    `json:"head_sha"`
				URL        string    `json:"html_url"`
				Completed  time.Time `json:"completed_at"`
			} `json:"check_runs"`
		}
		endpoint := path + "/commits/" + pr.Head.SHA + fmt.Sprintf("/check-runs?filter=all&per_page=%d&page=%d", pageSize, page)
		if err = r.get(ctx, endpoint, &checks); err != nil {
			return Batch{}, err
		}
		if checks.Runs == nil {
			return Batch{}, &Failure{"INVALID_RESPONSE"}
		}
		for _, entry := range checks.Runs {
			if entry.ID <= 0 || entry.HeadSHA != pr.Head.SHA || !checkURL(entry.URL, cfg.Repository) {
				return Batch{}, &Failure{"INVALID_RESPONSE"}
			}
			if entry.Status != "completed" {
				continue
			}
			if entry.Completed.IsZero() || entry.Conclusion == "" {
				return Batch{}, &Failure{"INVALID_RESPONSE"}
			}
			e := Observation{Kind: "check_run", SourceID: entry.ID, URL: entry.URL, Body: boundedText(entry.Name), State: entry.Conclusion, HeadSHA: entry.HeadSHA, UpdatedAt: entry.Completed.UTC()}
			e.identify()
			if err = batch.add(e); err != nil {
				return Batch{}, err
			}
		}
		if len(checks.Runs) < pageSize {
			break
		}
		if page == maxPages {
			return Batch{}, &Failure{"HISTORY_LIMIT"}
		}
	}
	return batch, nil
}

func (r reader) get(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com"+path, nil)
	if err != nil {
		return &Failure{"INVALID_REQUEST"}
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	resp, err := r.client.Do(req)
	if err != nil {
		return &Failure{"UNAVAILABLE"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		code := "UNAVAILABLE"
		switch resp.StatusCode {
		case 401:
			code = "CREDENTIAL_REJECTED"
		case 403, 429:
			code = "ACCESS_DENIED_OR_RATE_LIMITED"
		case 404:
			code = "SOURCE_UNAVAILABLE"
		case 301, 302, 303, 307, 308:
			code = "SOURCE_MOVED"
		}
		return &Failure{code}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || !utf8.Valid(raw) {
		return &Failure{"INVALID_RESPONSE"}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err = dec.Decode(dst); err != nil {
		return &Failure{"INVALID_RESPONSE"}
	}
	if dec.Decode(new(any)) != io.EOF {
		return &Failure{"INVALID_RESPONSE"}
	}
	return nil
}

func sourceURL(raw string, cfg Config) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil && u.RawQuery == "" && strings.EqualFold(u.Path, "/"+cfg.Repository+"/pull/"+fmt.Sprint(cfg.PullNumber))
}
func checkURL(raw, repository string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil && strings.HasPrefix(strings.ToLower(u.Path), "/"+strings.ToLower(repository)+"/")
}
func boundedText(text string) string {
	const limit = 16 << 10
	if len(text) <= limit {
		return text
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text + "\n[truncated; open the source URL for full text]"
}
