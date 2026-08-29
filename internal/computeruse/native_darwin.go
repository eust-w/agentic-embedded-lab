//go:build darwin && cgo

package computeruse

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>
#include <stdbool.h>

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
