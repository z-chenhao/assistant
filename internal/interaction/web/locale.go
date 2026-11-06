package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"regexp"
	"strings"
)

// A browser presentation preference, independent of model language and stored content.
const languageCookie = "steward-language"

//go:embed locales/zh-CN.json
var localeFiles embed.FS

var chinese = func() map[string]string {
	raw, err := localeFiles.ReadFile("locales/zh-CN.json")
	if err != nil {
		panic(err)
	}
	var values map[string]string
	if err = json.Unmarshal(raw, &values); err != nil {
		panic(err)
	}
	return values
}()

type translatedPattern struct {
	expression *regexp.Regexp
	text       string
}

var translatedPatterns = func() []translatedPattern {
	var result []translatedPattern
	for key, value := range chinese {
		if strings.Contains(key, "%d") {
			expression := "^" + strings.ReplaceAll(regexp.QuoteMeta(key), "%d", "([0-9]+)") + "$"
			result = append(result, translatedPattern{regexp.MustCompile(expression), value})
		}
	}
	return result
}()

func requestLanguage(r *http.Request) string {
	if cookie, err := r.Cookie(languageCookie); err == nil && cookie.Value == "zh-CN" {
		return "zh-CN"
	}
	return "en"
}

// Only call for application-authored copy. Never translate messages, filenames,
// arguments, source evidence, IDs or form values using this lookup.
func translate(language, text string) string {
	if language != "zh-CN" {
		return text
	}
	if value, ok := chinese[text]; ok {
		return value
	}
	for _, pattern := range translatedPatterns {
		if values := pattern.expression.FindStringSubmatch(text); values != nil {
			result := pattern.text
			for _, value := range values[1:] {
				result = strings.Replace(result, "%d", value, 1)
			}
			return result
		}
	}
	return text
}
func localizedLayout(language string) *template.Template {
	if language == "zh-CN" {
		return template.Must(chineseLayoutSource.Clone())
	}
	return template.Must(layoutSource.Clone())
}
func (s *Server) setLanguage(w http.ResponseWriter, r *http.Request) error {
	language := r.FormValue("language")
	if language != "en" && language != "zh-CN" {
		if r.Header.Get("Accept") == "application/json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			jsonResult(w, map[string]string{"error": "invalid_language"})
			return nil
		}
		p := page{View: "settings", Tab: "/settings", Title: "Preferences", Settings: &settingsView{Section: "preferences"}, Error: "Invalid language. Choose English or Simplified Chinese."}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Language", requestLanguage(r))
		w.WriteHeader(http.StatusBadRequest)
		s.render(w, r, p)
		return nil
	}
	http.SetCookie(w, &http.Cookie{Name: languageCookie, Value: language, Path: "/", MaxAge: 365 * 24 * 60 * 60, SameSite: http.SameSiteStrictMode})
	if r.Header.Get("Accept") == "application/json" {
		jsonResult(w, map[string]string{"language": language})
		return nil
	}
	http.Redirect(w, r, "/settings/preferences?notice=language-saved", http.StatusSeeOther)
	return nil
}
