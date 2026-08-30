package main

import "testing"

func TestMCPExposesOnlyReadOnlyAcceptanceTools(t *testing.T) {
	definitions := toolDefinitions()
	if len(definitions) != 3 {
		t.Fatal(definitions)
	}
	for _, item := range definitions {
		if item["name"] == "shell" {
			t.Fatal("shell exposed")
		}
	}
}
