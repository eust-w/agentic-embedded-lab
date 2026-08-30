package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/containerlab"
	"os"
	"path/filepath"
	"time"
)

func main() {
	workspace := flag.String("workspace", ".", "workspace")
	experiment := flag.String("experiment", "", "v2 experiment")
	system := flag.String("system", "", "v2 system")
	revision := flag.String("revision", "working-tree", "source revision")
	timeout := flag.Duration("timeout", 30*time.Minute, "run timeout")
	buildCase := flag.Int("build-case", 0, "compile a firmware mechanism before running")
	variant := flag.String("variant", "fixed", "firmware variant")
	finalizeMCUboot := flag.Bool("finalize-mcuboot", false, "regenerate MCUboot merged HEX from existing signed artifacts")
	flag.Parse()
	root, err := filepath.Abs(*workspace)
	fatal(err)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	lab := containerlab.Lab{Workspace: root, WorkerBinary: filepath.Join(root, ".ael", "container-bin", "ael-backend"), RuntimeRoot: filepath.Join(root, ".ael", "container-runs"), Images: containerlab.DefaultImages()}
	if *finalizeMCUboot {
		fatal(lab.FinalizeMCUbootArtifacts(*variant))
		fmt.Println("finalized mcuboot-" + *variant)
		return
	}
	if *buildCase > 0 {
		fatal(lab.BuildFirmware(ctx, *buildCase, *variant))
		if *experiment == "" {
			fmt.Printf("built case%02d-%s\n", *buildCase, *variant)
			return
		}
	}
	if *experiment == "" || *system == "" {
		fatal(fmt.Errorf("experiment and system are required"))
	}
	bundle, evidence, runErr := lab.Run(ctx, *experiment, *system, *revision)
	passed := runErr == nil
	for _, assertion := range bundle.Assertions {
		passed = passed && assertion.Passed
	}
	result := map[string]any{"run_id": bundle.RunID, "passed": passed, "trace_sha256": bundle.TraceSHA256, "assertions": bundle.Assertions, "evidence_path": evidence, "fidelity": bundle.Fidelity}
	if runErr != nil {
		result["error"] = runErr.Error()
	}
	payload, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(payload))
	if !passed {
		os.Exit(2)
	}
}
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
