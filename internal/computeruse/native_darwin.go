//go:build darwin && cgo

package computeruse

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics -framework AppKit -framework Foundation -framework ImageIO -framework ScreenCaptureKit
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>
#include <stdbool.h>
#include <stdlib.h>

char *aether_frontmost_bundle_id(void);
char *aether_ax_snapshot_json(int limit);
bool aether_focused_secure(void);
unsigned char *aether_screen_png(size_t *length);
void aether_free_buffer(void *buffer);

static bool aether_ax_trusted(bool prompt) {
  const void *keys[] = { kAXTrustedCheckOptionPrompt };
  const void *values[] = { prompt ? kCFBooleanTrue : kCFBooleanFalse };
  CFDictionaryRef options = CFDictionaryCreate(NULL, keys, values, 1,
      &kCFCopyStringDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
  bool trusted = AXIsProcessTrustedWithOptions(options);
  CFRelease(options);
  return trusted;
}

static bool aether_screen_trusted(bool prompt) {
  if (CGPreflightScreenCaptureAccess()) return true;
  return prompt ? CGRequestScreenCaptureAccess() : false;
}

static int aether_click(double x, double y) {
  CGPoint point = CGPointMake(x, y);
  CGEventRef down = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, point, kCGMouseButtonLeft);
  CGEventRef up = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, point, kCGMouseButtonLeft);
  if (!down || !up) return 1;
  CGEventPost(kCGHIDEventTap, down);
  CGEventPost(kCGHIDEventTap, up);
  CFRelease(down);
  CFRelease(up);
  return 0;
}

static int aether_type(const UniChar *characters, size_t length) {
  CGEventRef event = CGEventCreateKeyboardEvent(NULL, 0, true);
  if (!event) return 1;
  CGEventKeyboardSetUnicodeString(event, length, characters);
  CGEventPost(kCGHIDEventTap, event);
  CFRelease(event);
  return 0;
}
*/
import "C"

import (
	"errors"
	"unicode/utf16"
	"unsafe"
)

type MacNative struct{}

func NewNative() Native { return MacNative{} }

func (MacNative) AccessibilityTrusted(prompt bool) bool {
	return bool(C.aether_ax_trusted(C.bool(prompt)))
}

func (MacNative) ScreenRecordingTrusted(prompt bool) bool {
	return bool(C.aether_screen_trusted(C.bool(prompt)))
}

func (MacNative) FrontmostBundleID() (string, error) {
	value := C.aether_frontmost_bundle_id()
	if value == nil {
		return "", errors.New("unable to identify the frontmost application")
	}
	defer C.aether_free_buffer(unsafe.Pointer(value))
	return C.GoString(value), nil
}

func (MacNative) FocusedElementSecure() bool { return bool(C.aether_focused_secure()) }

func (MacNative) ElementTree(limit int) ([]byte, error) {
	value := C.aether_ax_snapshot_json(C.int(limit))
	if value == nil {
		return nil, errors.New("unable to read the Accessibility element tree")
	}
	defer C.aether_free_buffer(unsafe.Pointer(value))
	return []byte(C.GoString(value)), nil
}

func (MacNative) Screenshot() ([]byte, error) {
	var length C.size_t
	value := C.aether_screen_png(&length)
	if value == nil || length == 0 {
		return nil, errors.New("unable to capture the main display")
	}
	defer C.aether_free_buffer(unsafe.Pointer(value))
	return C.GoBytes(unsafe.Pointer(value), C.int(length)), nil
}

func (MacNative) Click(x, y float64) error {
	if C.aether_click(C.double(x), C.double(y)) != 0 {
		return errors.New("failed to create macOS mouse event")
	}
	return nil
}

func (MacNative) Type(text string) error {
	characters := utf16.Encode([]rune(text))
	if len(characters) == 0 {
		return nil
	}
	if C.aether_type((*C.UniChar)(unsafe.Pointer(&characters[0])), C.size_t(len(characters))) != 0 {
		return errors.New("failed to create macOS keyboard event")
	}
	return nil
}
