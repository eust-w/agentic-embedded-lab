//go:build !darwin || !cgo

package computeruse

import "errors"

type unavailableNative struct{}

func NewNative() Native                                    { return unavailableNative{} }
func (unavailableNative) AccessibilityTrusted(bool) bool   { return false }
func (unavailableNative) ScreenRecordingTrusted(bool) bool { return false }
func (unavailableNative) FrontmostBundleID() (string, error) {
	return "", errors.New("Computer Use is unavailable")
}
func (unavailableNative) FocusedElementSecure() bool { return true }
func (unavailableNative) ElementTree(int) ([]byte, error) {
	return nil, errors.New("Computer Use is unavailable")
}
func (unavailableNative) Screenshot() ([]byte, error) {
	return nil, errors.New("Computer Use is unavailable")
}
func (unavailableNative) Click(float64, float64) error {
	return errors.New("Computer Use is unavailable")
}
func (unavailableNative) Type(string) error { return errors.New("Computer Use is unavailable") }
