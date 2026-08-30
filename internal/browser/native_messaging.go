package browser

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	NativeHostName       = "dev.aether.desktop"
	ChromeExtensionID    = "nkpiamfhpapfmhgjallhkoapfpogldbe"
	maxNativeMessageSize = 1024 * 1024
)

type NativeMessage struct {
	Type    string         `json:"type"`
	ID      string         `json:"id"`
	TabID   int64          `json:"tab_id,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type NativeResponse struct {
	ID      string         `json:"id"`
	OK      bool           `json:"ok"`
	Error   string         `json:"error,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type ChromeSnapshot struct {
	ID         string    `json:"id"`
	TabID      int64     `json:"tab_id"`
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	DOM        string    `json:"dom"`
	CapturedAt time.Time `json:"captured_at"`
}

type ChromeSessionStore struct {
	mu     sync.RWMutex
	latest ChromeSnapshot
}

func (s *ChromeSessionStore) Ingest(message NativeMessage) (ChromeSnapshot, error) {
	if message.Type != "snapshot" || strings.TrimSpace(message.ID) == "" || message.TabID <= 0 {
		return ChromeSnapshot{}, errors.New("valid snapshot id and tab id are required")
	}
	rawURL, _ := message.Payload["url"].(string)
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "file") {
		return ChromeSnapshot{}, errors.New("snapshot URL scheme is not allowed")
	}
	dom, _ := message.Payload["dom"].(string)
	if len(dom) > 900_000 {
		return ChromeSnapshot{}, errors.New("snapshot DOM exceeds the native messaging limit")
	}
	snapshot := ChromeSnapshot{ID: message.ID, TabID: message.TabID, URL: rawURL, DOM: dom, CapturedAt: time.Now().UTC()}
	snapshot.Title, _ = message.Payload["title"].(string)
	s.mu.Lock()
	s.latest = snapshot
	s.mu.Unlock()
	return snapshot, nil
}

func (s *ChromeSessionStore) Latest() (ChromeSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest, s.latest.ID != ""
}

func ReadNativeMessage(reader io.Reader) (NativeMessage, error) {
	var size uint32
	if err := binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return NativeMessage{}, err
	}
	if size == 0 || size > maxNativeMessageSize {
		return NativeMessage{}, fmt.Errorf("native message size %d is outside the allowed range", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return NativeMessage{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	var message NativeMessage
	if err := decoder.Decode(&message); err != nil {
		return NativeMessage{}, err
	}
	return message, nil
}

func WriteNativeResponse(writer io.Writer, response NativeResponse) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if len(payload) > maxNativeMessageSize {
		return errors.New("native response exceeds the allowed size")
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(len(payload))); err != nil {
		return err
	}
	_, err = writer.Write(payload)
	return err
}

func NativeHostManifest(executable string) map[string]any {
	return map[string]any{
		"name":            NativeHostName,
		"description":     "Aether Desktop explicit Chrome tab bridge",
		"path":            executable,
		"type":            "stdio",
		"allowed_origins": []string{"chrome-extension://" + ChromeExtensionID + "/"},
	}
}
