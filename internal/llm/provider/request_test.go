package provider

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestResponsesNamespaceHasRequiredDescription(t *testing.T) {
	wire, err := BuildRequest(Model{ID: "model", API: OpenAIResponses, BaseURL: "https://api.openai.com/v1"}, Context{Tools: []Tool{{Name: "echo", Namespace: "steward", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	tools := wire.Body["tools"].([]any)
	namespace := tools[0].(map[string]any)
	if namespace["type"] != "namespace" || namespace["description"] == "" || namespace["description"] == nil {
		t.Fatal("public Responses API rejects namespace tools without description", namespace)
	}
}

func TestExplicitCloudEndpointsArePreserved(t *testing.T) {
	for _, api := range []API{BedrockConverse, GoogleVertex, AzureResponses} {
		for _, endpoint := range []string{"https://private.example.test/v1", "http://127.0.0.1:9876/v1"} {
			model := Model{ID: "model", API: api, BaseURL: endpoint}
			options := Options{Project: "project", Location: "europe-west1", AWS: AWSCredentials{Region: "us-west-2"}, Env: map[string]string{"AZURE_OPENAI_ENDPOINT": "https://public.example.test"}}
			wire, err := BuildRequest(model, Context{}, options)
			if err != nil {
				t.Fatal(err)
			}
			actual, _ := url.Parse(wire.URL)
			configured, _ := url.Parse(endpoint)
			if actual.Scheme != configured.Scheme || actual.Host != configured.Host {
				t.Fatal("explicit endpoint was replaced", api, wire.URL)
			}
		}
	}
}

func TestAzureDefaultEndpointFromEnvironment(t *testing.T) {
	wire, err := BuildRequest(Model{ID: "model", API: AzureResponses}, Context{}, Options{Env: map[string]string{"AZURE_OPENAI_ENDPOINT": "https://azure.example.test"}})
	if err != nil || wire.URL != "https://azure.example.test/openai/v1/responses" {
		t.Fatal("missing default Azure endpoint", wire.URL, err)
	}
}

func TestCloudDefaultEndpointsResolveRegion(t *testing.T) {
	for _, test := range []struct {
		api  API
		host string
	}{{BedrockConverse, "bedrock-runtime.us-west-2.amazonaws.com"}, {GoogleVertex, "europe-west1-aiplatform.googleapis.com"}} {
		wire, err := BuildRequest(Model{ID: "model", API: test.api}, Context{}, Options{Project: "project", Location: "europe-west1", AWS: AWSCredentials{Region: "us-west-2"}})
		if err != nil {
			t.Fatal(err)
		}
		actual, _ := url.Parse(wire.URL)
		if actual.Host != test.host {
			t.Fatal("default endpoint lost region selection", wire.URL)
		}
	}
}
