package provider

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Frozen AWS SigV4 reference vectors use synthetic credentials and a fixed
// timestamp. They cover both a plain model ID and an encoded inference ARN.
func TestBedrockSigningVectors(t *testing.T) {
	cases := map[string]string{"/model/test/converse-stream": "fccfe757ebf33c277b2060445ef57295c0f4408c6fb5fda000ed371acb7e9328", "/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A123%3Ainference-profile%2Fmodel/converse-stream": "b35a393ab751ea5dfee168c957a24da8cf62a206cff7b9dd71785bb9dd20a0e0"}
	for path, want := range cases {
		request, _ := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com"+path, strings.NewReader("{}"))
		request.Header.Set("Content-Type", "application/json")
		if err := signAWS(request, []byte("{}"), AWSCredentials{AccessKey: "EXAMPLE", SecretKey: "fixture-secret", Region: "us-east-1"}, time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(request.Header.Get("Authorization"), "Signature="+want) {
			t.Error(path, request.Header.Get("Authorization"))
		}
	}
}
