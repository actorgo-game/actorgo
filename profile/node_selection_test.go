package cprofile

import "testing"

func TestGetNodeWithConfigMatchesNodeID(t *testing.T) {
	config := Wrap(map[string]any{
		"node": map[string]any{
			"5": []any{
				map[string]any{"node_id": "1.10001.5.1", "__settings__": map[string]any{"snowflake_node": 10001}, "enable": true},
				map[string]any{"node_id": "1.10001.5.2", "__settings__": map[string]any{"snowflake_node": 10002}, "enable": true},
			},
		},
	})
	node, err := GetNodeWithConfig(config, "1.10001.5.2", "5")
	if err != nil {
		t.Fatal(err)
	}
	if got := node.Settings().GetInt64("snowflake_node"); got != 10002 {
		t.Fatalf("selected the wrong node settings: %d", got)
	}
}
