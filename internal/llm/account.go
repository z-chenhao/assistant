package llm

import "time"

// OAuthCredential crosses only trusted Auth/SecretStore/Bridge boundaries.
// It must never be emitted as a UI event, diagnostic, usage record or error.
type OAuthCredential struct {
	ClientID     string    `json:"client_id"`
	SubjectHash  string    `json:"subject_hash"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	Scopes       []string  `json:"scopes"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type AccountRegistration struct {
	ClientID    string `json:"client_id"`
	SubjectHash string `json:"subject_hash"`
	AuthEpoch   int64  `json:"auth_epoch"`
}
type AccountLogin struct {
	Status     string           `json:"status"`
	Events     []AuthEvent      `json:"events"`
	Credential *OAuthCredential `json:"credential,omitempty"`
}
