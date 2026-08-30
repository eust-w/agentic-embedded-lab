package packaging

import (
	"debug/macho"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type Report struct {
	Profile           string   `json:"profile"`
	App               string   `json:"app"`
	Passed            bool     `json:"passed"`
	Checks            []Check  `json:"checks"`
	HardwareValidated bool     `json:"hardware_validated"`
	Limitations       []string `json:"limitations"`
}

func CheckBundle(app string, release bool) Report {
	profile := "development"
	if release {
		profile = "release"
	}
	report := Report{Profile: profile, App: app, HardwareValidated: false, Limitations: []string{"Desktop packaging does not provide hardware validation or a signed Validation Envelope."}}
	add := func(name string, passed bool, detail string) {
		report.Checks = append(report.Checks, Check{Name: name, Passed: passed, Detail: detail})
	}
	infoPath := filepath.Join(app, "Contents", "Info.plist")
	info, infoErr := os.ReadFile(infoPath)
	add("bundle:info-plist", infoErr == nil && strings.Contains(string(info), "dev.aether.desktop") && strings.Contains(string(info), "14.0"), infoPath)
	for _, name := range []string{"Aether Desktop", "aetherd", "ael-backend", "aether-chrome-host", "aether-mcp"} {
		path := filepath.Join(app, "Contents", "MacOS", name)
		file, err := macho.Open(path)
		passed := err == nil && file.Cpu == macho.CpuArm64
		if file != nil {
			_ = file.Close()
		}
		add("binary:"+name, passed, path)
	}
	extensionPath := filepath.Join(app, "Contents", "Resources", "ChromeExtension", "manifest.json")
	extension, extensionErr := os.ReadFile(extensionPath)
	var extensionManifest map[string]any
	if extensionErr == nil {
		extensionErr = json.Unmarshal(extension, &extensionManifest)
	}
	add("browser:chrome-extension", extensionErr == nil && extensionManifest["manifest_version"] == float64(3), extensionPath)
	hostTemplate := filepath.Join(app, "Contents", "Resources", "dev.aether.desktop.json.in")
	host, hostErr := os.ReadFile(hostTemplate)
	add("browser:native-host-manifest", hostErr == nil && strings.Contains(string(host), "nkpiamfhpapfmhgjallhkoapfpogldbe"), hostTemplate)
	launchAgent := filepath.Join(app, "Contents", "Library", "LaunchAgents", "dev.aether.desktop.daemon.plist")
	launchAgentData, launchAgentErr := os.ReadFile(launchAgent)
	add("automation:launch-agent", launchAgentErr == nil && strings.Contains(string(launchAgentData), "Contents/MacOS/aetherd"), launchAgent)
	for name, path := range map[string]string{
		"browser:bundled-chromium": filepath.Join(app, "Contents", "Resources", "Chromium.app", "Contents", "MacOS", "Chromium"),
		"update:sparkle":           filepath.Join(app, "Contents", "Frameworks", "Sparkle.framework"),
	} {
		_, err := os.Stat(path)
		add(name, err == nil, path)
	}
	if runtime.GOOS == "darwin" {
		output, err := exec.Command("codesign", "--verify", "--deep", "--strict", app).CombinedOutput()
		add("signature:codesign", err == nil, strings.TrimSpace(string(output)))
	} else {
		add("signature:codesign", false, "codesign is available only on macOS")
	}
	if release {
		_, xcodeErr := exec.LookPath("xcodebuild")
		fullXcode := xcodeErr == nil && exec.Command("xcodebuild", "-version").Run() == nil
		add("toolchain:full-xcode", fullXcode, "xcodebuild -version")
		add("signature:developer-id", os.Getenv("AETHER_SIGN_IDENTITY") != "", "AETHER_SIGN_IDENTITY")
		add("signature:notary-profile", os.Getenv("AETHER_NOTARY_PROFILE") != "", "AETHER_NOTARY_PROFILE")
		add("update:feed-url", os.Getenv("AETHER_SPARKLE_FEED_URL") != "", "AETHER_SPARKLE_FEED_URL")
		add("update:public-key", os.Getenv("AETHER_SPARKLE_PUBLIC_KEY") != "", "AETHER_SPARKLE_PUBLIC_KEY")
	}
	report.Passed = true
	for _, check := range report.Checks {
		report.Passed = report.Passed && check.Passed
	}
	return report
}

func (r Report) Error() error {
	if r.Passed {
		return nil
	}
	var failures []string
	for _, check := range r.Checks {
		if !check.Passed {
			failures = append(failures, fmt.Sprintf("%s (%s)", check.Name, check.Detail))
		}
	}
	return fmt.Errorf("desktop package gate failed: %s", strings.Join(failures, "; "))
}
