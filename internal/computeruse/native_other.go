//go:build !darwin || !cgo

package computeruse

import "errors"

type unavailableNative struct{}

func NewNative() Native                                    { return unavailableNative{} }
func (unavailableNative) AccessibilityTrusted(bool) bool   { return false }
func (unavailableNative) ScreenRecordingTrusted(bool) bool { return false }
func (unavailableNative) Click(float64, float64) error {
	return errors.New("Computer Use is unavailable")
}
func (unavailableNative) Type(string) error { return errors.New("Computer Use is unavailable") }
