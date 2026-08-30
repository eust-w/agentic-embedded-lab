package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/benchmark"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/containerlab"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/fmi"
)

func main() {
	workspace := flag.String("workspace", ".", "workspace")
	systemPath := flag.String("system", "benchmarks/v2/systems/five-domain-fixed.yaml", "v2 system")
	experimentPath := flag.String("experiment", "benchmarks/v2/experiments/24-antenna-cross-domain-fixed.yaml", "v2 experiment providing FMI start values")
	coordinator := flag.String("omsimulator", "scripts/omsimulator-python-container", "OMSimulator Python container wrapper")
	flag.Parse()
	root, err := filepath.Abs(*workspace)
	fatal(err)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	system, err := ael.LoadSystem(root, *systemPath)
	fatal(err)
	experiment, err := ael.LoadExperiment(root, *experimentPath)
	fatal(err)
	starts := map[string]float64{}
	for _, stimulus := range experiment.Stimuli {
		starts[stimulus.Target] = stimulus.Value
	}
	startSpecs := map[string]map[string]any{}
	for _, component := range system.Components {
		for _, port := range component.Ports {
			if value, ok := starts[component.ID+"."+port.Name]; ok {
				startSpecs[component.ID+"."+port.Name] = map[string]any{"type": port.Type, "value": value}
			}
		}
	}
	lab := containerlab.Lab{Workspace: root, WorkerBinary: filepath.Join(root, ".ael", "container-bin", "ael-backend"), RuntimeRoot: filepath.Join(root, ".ael", "container-runs"), Images: containerlab.DefaultImages()}
	fatal(buildLinuxWorker(ctx, root, lab.WorkerBinary))
	fatal(lab.RewriteFirmware(root, &system))
	build := filepath.Join(root, ".ael", "build", "fmi-v2")
	fatal(os.MkdirAll(build, 0o700))
	fatal(buildLinuxProxies(ctx, root, build))
	fmus := map[string]string{}
	for _, component := range system.Components {
		proxy, ok := fmi.ProxyName(component.Backend)
		if !ok {
			fatal(fmt.Errorf("no FMI proxy for backend %s", component.Backend))
		}
		ports := filepath.Join(build, component.ID+"-ports.json")
		portValues := make([]map[string]any, 0, len(component.Ports))
		for _, port := range component.Ports {
			value := map[string]any{"name": port.Name, "direction": port.Direction, "data_type": port.Type, "unit": port.Unit}
			if start, ok := starts[component.ID+"."+port.Name]; ok && port.Direction == "input" {
				value["start"] = start
			}
			portValues = append(portValues, value)
		}
		data, _ := json.MarshalIndent(portValues, "", "  ")
		fatal(os.WriteFile(ports, append(data, '\n'), 0o600))
		output := filepath.Join(build, component.ID+".fmu")
		platformTag := "linux64"
		if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			platformTag = "linux32"
		}
		fatal(run(ctx, root, "python3", "scripts/package_fmu.py", "--proxy", proxy, "--library", filepath.Join(build, "linux64", proxy+".so"), "--ports", ports, "--output", output, "--platform-tag", platformTag))
		fmus[component.ID] = output
	}
	ssp := filepath.Join(build, "five-domain-v2.ssp")
	fatal(fmi.ExportSSP(system, fmus, ssp))
	loaded, err := fmi.LoadSSP(ssp)
	fatal(err)
	if len(loaded.Components) != len(system.Components) || len(loaded.Connections) != len(system.Connections) {
		fatal(errors.New("exported SSP topology does not match the v2 system"))
	}

	instances := map[string]*fmi.Instance{}
	adapters := []ael.Adapter{}
	for _, component := range system.Components {
		adapter, _, err := lab.Adapter(root, component)
		fatal(err)
		fatal(adapter.Prepare(ctx, component, 1024))
		adapters = append(adapters, adapter)
		variables := map[uint32]fmi.Variable{}
		for index, port := range component.Ports {
			kind := map[string]fmi.ValueType{"real": fmi.Real, "integer": fmi.Integer, "boolean": fmi.Boolean}[port.Type]
			if kind == 0 {
				fatal(fmt.Errorf("unsupported FMI port type %s", port.Type))
			}
			reference := uint32(index + 1)
			variables[reference] = fmi.Variable{Reference: reference, Name: port.Name, Type: kind, Direction: port.Direction}
		}
		instances[component.ID] = &fmi.Instance{Adapter: adapter, Variables: variables}
	}
	defer func() {
		for _, adapter := range adapters {
			_ = adapter.Shutdown(context.Background())
		}
	}()
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	fatal(err)
	bridge := &fmi.Bridge{Instances: instances}
	bridgeErr := make(chan error, 1)
	go func() { bridgeErr <- bridge.Serve(ctx, listener) }()
	port := listener.Addr().(*net.TCPAddr).Port
	containerHost, err := resolveContainerHost(ctx, root)
	fatal(err)
	environment := append(os.Environ(), "AEL_FMI_CONTAINER_HOST="+containerHost, "AEL_OMSIMULATOR_IMAGE=ael-openmodelica:local")
	endpoint := fmt.Sprintf("tcp://%s:%d", containerHost, port)
	for _, component := range system.Components {
		environment = append(environment, "AEL_FMI_SOCKET_"+strings.ToUpper(strings.ReplaceAll(string(component.Backend), "-", "_"))+"="+endpoint)
	}
	result := filepath.Join(build, "five-domain-v2-result.csv")
	logPath := filepath.Join(build, "five-domain-v2-omsimulator.log")
	startsPath := filepath.Join(build, "five-domain-v2-starts.json")
	startsData, _ := json.MarshalIndent(startSpecs, "", "  ")
	fatal(os.WriteFile(startsPath, append(startsData, '\n'), 0o600))
	coordinatorPath := *coordinator
	if !filepath.IsAbs(coordinatorPath) {
		coordinatorPath = filepath.Join(root, coordinatorPath)
	}
	command := exec.CommandContext(ctx, coordinatorPath, filepath.Join(root, "scripts", "run_omsimulator_v2.py"), "--ssp", ssp, "--system-id", system.ID, "--starts", startsPath, "--result", result, "--log", logPath, "--stop-time", "0.006")
	command.Dir, command.Env = root, environment
	output, err := command.CombinedOutput()
	if err != nil {
		fatal(fmt.Errorf("OMSimulator: %w: %s", err, tail(output, 4000)))
	}
	logData, _ := os.ReadFile(logPath)
	if containsCoordinatorError(string(logData)) {
		fatal(fmt.Errorf("OMSimulator log contains an error: %s", tail(logData, 4000)))
	}
	nonzero, err := nonzeroResults(result)
	fatal(err)
	if len(nonzero) == 0 {
		fatal(errors.New("FMI acceptance produced no non-zero output"))
	}
	cancel()
	_ = listener.Close()
	select {
	case err := <-bridgeErr:
		if err != nil {
			fatal(err)
		}
	case <-time.After(time.Second):
	}

	revision := gitRevision(root)
	evidencePath := filepath.Join(root, "acceptance", "v2", "evidence", "fmi-five-domain.json")
	inputHashes := map[string]string{}
	for _, path := range []string{*systemPath, *experimentPath, "native/fmi-proxies/ael_fmi_proxy.cpp", "native/fmi-proxies/CMakeLists.txt", "scripts/package_fmu.py"} {
		hash, err := benchmark.FileSHA256(filepath.Join(root, path))
		fatal(err)
		inputHashes[path] = hash
	}
	sspHash, _ := benchmark.FileSHA256(ssp)
	resultHash, _ := benchmark.FileSHA256(result)
	evidence := map[string]any{"api_version": ael.APIVersion, "status": "passed", "source_revision": revision, "system": *systemPath, "experiment": *experimentPath, "input_hashes": inputHashes, "ssp_sha256": sspHash, "result_sha256": resultHash, "nonzero_outputs": nonzero, "components": len(system.Components), "connections": len(system.Connections), "hardware_validated": false, "limitations": []string{"FMI 2.0 functional exchange only; no calibrated hardware equivalence."}, "created_at": time.Now().UTC()}
	evidenceData, _ := json.MarshalIndent(evidence, "", "  ")
	fatal(os.MkdirAll(filepath.Dir(evidencePath), 0o700))
	fatal(os.WriteFile(evidencePath, append(evidenceData, '\n'), 0o600))
	evidenceHash, _ := benchmark.FileSHA256(evidencePath)
	fatal(updateManifest(root, revision, evidenceHash))
	fmt.Printf("fmi components=%d connections=%d nonzero=%d passed=true\n", len(system.Components), len(system.Connections), len(nonzero))
}

func buildLinuxProxies(ctx context.Context, root, build string) error {
	workerBuild := filepath.Join(build, "linux64")
	command := fmt.Sprintf("cmake -S native/fmi-proxies -B %s && cmake --build %s --parallel", shellPath(workerBuild), shellPath(workerBuild))
	return run(ctx, root, "docker", "run", "--rm", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=512", "--memory=4g", "--cpus=2", "--tmpfs=/tmp:rw,exec,nosuid,nodev,size=1g", fmt.Sprintf("--user=%d:%d", os.Getuid(), os.Getgid()), "--mount=type=bind,src="+root+",dst="+root, "--env=HOME=/tmp", "--workdir="+root, "--entrypoint=sh", "ael-openmodelica:local", "-c", command)
}

func buildLinuxWorker(ctx context.Context, root, output string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", output, "./cmd/ael-backend")
	command.Dir = root
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	data, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build linux/amd64 worker: %w: %s", err, tail(data, 4000))
	}
	return nil
}

func resolveContainerHost(ctx context.Context, root string) (string, error) {
	command := exec.CommandContext(ctx, "colima", "ssh", "--", "getent", "hosts", "host.lima.internal")
	command.Dir = root
	data, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve Colima host gateway: %w: %s", err, tail(data, 1000))
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 || net.ParseIP(fields[0]) == nil {
		return "", errors.New("Colima host gateway did not resolve to an IP address")
	}
	return fields[0], nil
}

func updateManifest(root, revision, evidenceHash string) error {
	path := filepath.Join(root, "acceptance", "v2", "simulation.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest benchmark.AcceptanceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	entries := manifest.Entries[:0]
	for _, entry := range manifest.Entries {
		if entry.Name != "fmi:five-domain" {
			entries = append(entries, entry)
		}
	}
	manifest.Entries = append(entries, benchmark.AcceptanceEntry{Name: "fmi:five-domain", Status: "passed", EvidencePath: "acceptance/v2/evidence/fmi-five-domain.json", EvidenceSHA256: evidenceHash, Limitations: []string{"FMI 2.0 functional exchange only; no calibrated hardware equivalence."}})
	manifest.SourceRevision = revision
	manifest.CreatedAt = time.Now().UTC()
	data, _ = json.MarshalIndent(manifest, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func nonzeroResults(path string) (map[string]float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, errors.New("OMSimulator result has no data")
	}
	result := map[string]float64{}
	last := rows[len(rows)-1]
	for index, name := range rows[0] {
		if index >= len(last) || strings.EqualFold(name, "time") || strings.EqualFold(name, "time [s]") {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(last[index]), 64)
		if err == nil && value != 0 {
			result[name] = value
		}
	}
	return result, nil
}
func containsCoordinatorError(value string) bool {
	for _, line := range strings.Split(value, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "| error:") || strings.Contains(lower, "| fatal:") {
			return true
		}
	}
	return false
}
func run(ctx context.Context, directory, executable string, arguments ...string) error {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", executable, err, tail(output, 4000))
	}
	return nil
}
func tail(value []byte, limit int) string {
	if len(value) <= limit {
		return string(value)
	}
	return string(value[len(value)-limit:])
}
func shellPath(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func gitRevision(root string) string {
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return "unresolved"
	}
	return strings.TrimSpace(string(output))
}
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
