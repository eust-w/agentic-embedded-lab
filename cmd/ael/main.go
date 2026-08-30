package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	capability "github.com/eust-w/agentic-embedded-lab/internal/acceptance"
	"github.com/eust-w/agentic-embedded-lab/internal/packaging"
	"github.com/eust-w/agentic-embedded-lab/internal/release"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "release" && os.Args[2] == "check" {
		releaseCheck(os.Args[3:])
		return
	}
	if len(os.Args) >= 3 && os.Args[1] == "capabilities" && os.Args[2] == "list" {
		capabilityList(os.Args[3:])
		return
	}
	if len(os.Args) >= 4 && os.Args[1] == "capabilities" && os.Args[2] == "inspect" {
		capabilityInspect(os.Args[3], os.Args[4:])
		return
	}
	if len(os.Args) >= 3 && os.Args[1] == "acceptance" && os.Args[2] == "report" {
		acceptanceReport(os.Args[3:])
		return
	}
	if len(os.Args) >= 3 && os.Args[1] == "acceptance" && os.Args[2] == "run" {
		acceptanceRun(os.Args[3:])
		return
	}
	{
		usage()
		os.Exit(2)
	}
}

func releaseCheck(arguments []string) {
	flags := flag.NewFlagSet("ael release check", flag.ContinueOnError)
	profileName := flags.String("profile", string(release.Foundation), "foundation|simulation|software|production")
	workspace := flags.String("workspace", ".", "项目工作区")
	output := flags.String("output", "", "可选 JSON 报告路径")
	if err := flags.Parse(arguments); err != nil {
		os.Exit(2)
	}
	profile := release.Profile(*profileName)
	switch profile {
	case release.Foundation, release.Desktop, release.Agent, release.Simulation, release.SimulationExtensions, release.Software, release.DevelopmentPackage, release.Production:
	default:
		fatal(fmt.Errorf("未知发布门 profile %q", *profileName))
	}
	result, err := release.Check(*workspace, profile)
	if err != nil {
		fatal(err)
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err)
	}
	if *output != "" {
		path, err := filepath.Abs(*output)
		if err != nil {
			fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
			fatal(err)
		}
	}
	fmt.Println(string(payload))
	if !result.Passed {
		os.Exit(2)
	}
}

func capabilityList(arguments []string) {
	root := workspaceFlag("capabilities list", arguments)
	values, err := capability.List(root, gitRevision(root))
	if err != nil {
		fatal(err)
	}
	emit(values, "")
}
func capabilityInspect(id string, arguments []string) {
	root := workspaceFlag("capabilities inspect", arguments)
	value, err := capability.Inspect(root, gitRevision(root), id)
	if err != nil {
		fatal(err)
	}
	emit(value, "")
}
func acceptanceReport(arguments []string) {
	root := workspaceFlag("acceptance report", arguments)
	value, err := capability.Report(root, gitRevision(root))
	if err != nil {
		fatal(err)
	}
	emit(value, "")
}
func acceptanceRun(arguments []string) {
	flags := flag.NewFlagSet("acceptance run", flag.ExitOnError)
	profile := flags.String("profile", "software", "desktop|agent|simulation|software|development-package")
	workspace := flags.String("workspace", ".", "workspace")
	output := flags.String("output", "acceptance/v2/capability-report.json", "report path")
	_ = flags.Parse(arguments)
	root, err := filepath.Abs(*workspace)
	if err != nil {
		fatal(err)
	}
	passed := false
	switch *profile {
	case "desktop":
		result, err := release.Check(root, release.Desktop)
		if err != nil {
			fatal(err)
		}
		passed = result.Passed
	case "agent":
		result, err := release.Check(root, release.Agent)
		if err != nil {
			fatal(err)
		}
		passed = result.Passed
	case "software":
		result, err := release.Check(root, release.Software)
		if err != nil {
			fatal(err)
		}
		passed = result.Passed
	case "simulation":
		result, err := release.Check(root, release.Simulation)
		if err != nil {
			fatal(err)
		}
		passed = result.Passed
	case "development-package":
		passed = packaging.CheckBundle(filepath.Join(root, "build/bin/Aether Desktop.app"), false).Passed
	default:
		fatal(fmt.Errorf("unknown acceptance profile %q", *profile))
	}
	report, err := capability.Report(root, gitRevision(root))
	if err != nil {
		fatal(err)
	}
	report["requested_profile"] = *profile
	report["profile_passed"] = passed
	target := *output
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	emit(report, target)
	if !passed {
		os.Exit(2)
	}
}

func workspaceFlag(name string, arguments []string) string {
	flags := flag.NewFlagSet(name, flag.ExitOnError)
	workspace := flags.String("workspace", ".", "workspace")
	_ = flags.Parse(arguments)
	root, err := filepath.Abs(*workspace)
	if err != nil {
		fatal(err)
	}
	return root
}
func gitRevision(root string) string {
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = root
	data, err := command.Output()
	if err != nil {
		return "unresolved"
	}
	return strings.TrimSpace(string(data))
}
func emit(value any, output string) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal(err)
	}
	if output != "" {
		if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(output, append(payload, '\n'), 0o600); err != nil {
			fatal(err)
		}
	}
	fmt.Println(string(payload))
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: ael release check|capabilities list|capabilities inspect|acceptance run|acceptance report")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
