package server

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWorkerExecutesAcceptanceTask(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	worker := &Worker{config: WorkerConfig{Workspace: root, ID: "test"}}
	result, err := worker.handle(context.Background(), Task{Attempt: 1, Payload: map[string]any{"kind": "acceptance", "profile": "foundation"}})
	if err != nil || result["status"] != "passed" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
