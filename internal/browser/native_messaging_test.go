package browser

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestNativeMessagingFrameAndSnapshotValidation(t *testing.T) {
	payload, _ := json.Marshal(NativeMessage{Type: "snapshot", ID: "capture-1", TabID: 7, Payload: map[string]any{"url": "https://example.com", "title": "示例", "dom": "<html></html>"}})
	var input bytes.Buffer
	_ = binary.Write(&input, binary.LittleEndian, uint32(len(payload)))
	_, _ = input.Write(payload)
	message, err := ReadNativeMessage(&input)
	if err != nil {
		t.Fatal(err)
	}
	store := &ChromeSessionStore{}
	snapshot, err := store.Ingest(message)
	if err != nil || snapshot.URL != "https://example.com" {
		t.Fatalf("snapshot: %#v %v", snapshot, err)
	}
	if latest, ok := store.Latest(); !ok || latest.ID != "capture-1" {
		t.Fatalf("latest snapshot missing: %#v", latest)
	}
}

func TestNativeMessagingRejectsOversizeAndUnsafeScheme(t *testing.T) {
	var input bytes.Buffer
	_ = binary.Write(&input, binary.LittleEndian, uint32(maxNativeMessageSize+1))
	if _, err := ReadNativeMessage(&input); err == nil {
		t.Fatal("oversize message was accepted")
	}
	store := &ChromeSessionStore{}
	if _, err := store.Ingest(NativeMessage{Type: "snapshot", ID: "x", TabID: 1, Payload: map[string]any{"url": "javascript:alert(1)"}}); err == nil {
		t.Fatal("unsafe URL scheme was accepted")
	}
}
