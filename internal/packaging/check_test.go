package packaging

import "testing"

func TestMissingBundleFailsClosed(t *testing.T) {
	report := CheckBundle(t.TempDir()+"/missing.app", false)
	if report.Passed || report.Error() == nil {
		t.Fatalf("missing bundle passed: %#v", report)
	}
}
