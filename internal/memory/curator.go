package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/z-chenhao/erii/internal/tool"
)

const ManageName = "builtin__memory__manage"
const ManageSchema = `{"type":"object","properties":{"operation":{"type":"string","enum":["remember","forget"]},"content":{"type":"string","minLength":1,"maxLength":4096},"memory_ref":{"type":"string","minLength":1,"maxLength":256},"category":{"type":"string","enum":["health","finance","identity","relationships","goals","learning","preferences","facts","all"]},"supersedes":{"type":"string","minLength":1,"maxLength":256}},"required":["operation"],"additionalProperties":false}`

type Config struct {
	EncryptionKeyRef string `yaml:"encryption_key_ref"`
}

func (c Config) Valid() bool {
	return c.EncryptionKeyRef != "" && len(c.EncryptionKeyRef) <= 256 && !strings.ContainsAny(c.EncryptionKeyRef, "\n\r\x00")
}

type Mutation struct {
	Operation  string `json:"operation"`
	Content    string `json:"content,omitempty"`
	MemoryRef  string `json:"memory_ref,omitempty"`
	Supersedes string `json:"supersedes,omitempty"`
	Category   string `json:"category,omitempty"`
}

func Definition() tool.Definition {
	return tool.Definition{Host: true, Spec: tool.Spec{Name: ManageName, Version: 1, Description: "Accept a user's explicit remember/correction or forget request in this user's Agent scope. Forget one memory_ref or opt out a category; remember with category resumes future extraction. Returns durable acceptance, not completed maintenance. Forget revokes unfinished context. Workers cannot write memory. Never submit secrets.", InputSchema: ManageSchema, Permission: "memory_control", ReplayPolicy: "safe", Risk: "low", TimeoutMS: 10000, OutputBytes: 4096, Provider: "builtin", ConflictPolicy: "none"}}
}

func Parse(raw string) (Mutation, bool) {
	var m Mutation
	if !tool.Validate(ManageSchema, raw) || json.Unmarshal([]byte(raw), &m) != nil || HasSecret(m.Content) {
		return m, false
	}
	validRef := func(v string) bool {
		return strings.HasPrefix(v, "memory:") && len(v) > 7 && !strings.ContainsAny(v, " /\\\n\r")
	}
	if m.Operation == "forget" {
		return m, (validRef(m.MemoryRef) && m.Category == "" || m.MemoryRef == "" && m.Category != "") && m.Content == "" && m.Supersedes == ""
	}
	if m.Category != "" {
		return m, m.Content == "" && m.MemoryRef == "" && m.Supersedes == ""
	}
	return m, strings.TrimSpace(m.Content) != "" && m.MemoryRef == "" && (m.Supersedes == "" || validRef(m.Supersedes))
}

// Categories are conservative opt-out filters, never diagnoses or new facts.
func Category(text string) string {
	s := strings.ToLower(text)
	for _, group := range []struct {
		name  string
		terms []string
	}{
		{"health", []string{"health", "sleep", "tired", "symptom", "diagnos", "medication", "睡", "病", "困", "药", "症状"}},
		{"finance", []string{"income", "salary", "asset", "employer options", "finance", "收支", "收入", "资产", "期权", "开销"}},
		{"identity", []string{"address", "passport", "birthday", "my age", "住址", "身份证", "护照", "生日", "我叫"}},
		{"relationships", []string{"family", "partner", "spouse", "家庭", "家人", "伴侣"}},
		{"goals", []string{"goal", "career", "independent development", "目标", "职业", "独立开发", "人生"}},
		{"learning", []string{"learning", "practic", "english", "学习", "练习", "英语"}},
		{"preferences", []string{"prefer", "like", "喜欢", "偏好"}},
	} {
		for _, term := range group.terms {
			if strings.Contains(s, term) {
				return group.name
			}
		}
	}
	return "facts"
}

var secretPattern = regexp.MustCompile(`(?i)(-----BEGIN .*PRIVATE KEY|\b(?:sk-[a-z0-9_-]{12,}|gh[pousr]_[a-z0-9]{12,}|eyJ[a-z0-9_-]{15,}\.)|(?:api[_ -]?key|access[_ -]?token|refresh[_ -]?token|password|密码|私钥|支付密钥)\s*[:=：]\s*\S+)`)

func HasSecret(s string) bool {
	lower := strings.ToLower(s)
	for _, term := range []string{"password", "api key", "api_key", "access token", "access_token", "refresh token", "refresh_token", "private key", "凭据", "密钥", "密码", "令牌"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return secretPattern.MatchString(s)
}
func Sensitive(s string) bool {
	s = strings.ToLower(s)
	for _, term := range []string{"health", "sleep", "symptom", "diagnos", "medication", "income", "salary", "asset", "option", "finance", "family", "address", "passport", "睡", "病", "症状", "药", "诊断", "收入", "资产", "期权", "家庭", "住址", "身份证", "护照"} {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}
func Fingerprint(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Extract is deliberately conservative: retain short attributed reports, never
// turn a question, Tool trace or a diagnosis inferred by a Worker into a fact.
// It is deterministic so retry/restart cannot change the candidate set.
func Extract(text, author string) []string {
	if HasSecret(text) || len(text) > 16<<10 {
		return nil
	}
	var out []string
	for _, sentence := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '。' || r == '!' || r == '！' }) {
		sentence = strings.TrimSpace(sentence)
		lower := strings.ToLower(sentence)
		if len(sentence) < 4 || len(sentence) > 1024 || strings.ContainsAny(sentence, "?？") {
			continue
		}
		attributed := false
		if author == "worker" && strings.HasPrefix(sentence, "User reported: ") {
			sentence = strings.TrimSpace(strings.TrimPrefix(sentence, "User reported: "))
			lower = strings.ToLower(sentence)
			attributed = true
		}
		personal := strings.Contains(sentence, "我") || strings.HasPrefix(lower, "i ") || strings.HasPrefix(lower, "my ")
		if (author != "user" && !attributed) || !personal {
			continue
		}
		if strings.Contains(lower, "forget") || strings.Contains(sentence, "忘记") || strings.Contains(sentence, "不要记") {
			continue
		}
		out = append(out, sentence)
		if len(out) == 8 {
			break
		}
	}
	return out
}

func Stable(s string) bool {
	s = strings.ToLower(s)
	for _, term := range []string{"maybe", "might", "perhaps", "explor", "today", "recent", "tired", "sleepy", "也许", "可能", "探索", "只是想", "今天", "近期", "好困", "昨晚", "you said", "you suggested", "apparently", "你说", "他说", "她说", "听说", "好像"} {
		if strings.Contains(s, term) {
			return false
		}
	}
	return true
}
