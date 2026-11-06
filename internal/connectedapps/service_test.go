package connectedapps

import (
	"encoding/json"
	"github.com/z-chenhao/assistant/internal/connector/composio"
	"testing"
)

func TestToolDescriptorCanonicalizationPreservesLargeIntegerEnums(t *testing.T) {
	one := composio.Tool{Slug: "SEND", Version: "20261101_00", Input: json.RawMessage(`{"properties":{"id":{"enum":[9007199254740992]}},"type":"object"}`)}
	reordered := one
	reordered.Input = json.RawMessage(`{ "type": "object", "properties": {"id": {"enum": [9007199254740992]}} }`)
	different := one
	different.Input = json.RawMessage(`{"properties":{"id":{"enum":[9007199254740993]}},"type":"object"}`)
	if ToolHash(one) != ToolHash(reordered) {
		t.Fatal("object order changed descriptor identity")
	}
	if ToolHash(one) == ToolHash(different) {
		t.Fatal("schema drift was hidden by numeric rounding")
	}
}
