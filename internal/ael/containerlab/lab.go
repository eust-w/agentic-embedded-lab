package containerlab

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
)

type Lab struct {
	Workspace, WorkerBinary, RuntimeRoot string
	Images                               map[ael.Backend]string
}

func DefaultImages() map[ael.Backend]string {
	return map[ael.Backend]string{ael.BackendZephyr: "ael-zephyr:local", ael.BackendRenode: "ael-renode:local", ael.BackendNgspice: "ael-ngspice:local", ael.BackendModelica: "ael-openmodelica:local", ael.BackendOMSimulator: "ael-openmodelica:local", ael.BackendNS3: "ael-ns3:local", ael.BackendOpenEMS: "ael-openems:local", ael.BackendVerilator: "ael-verilator:local"}
}
func (l Lab) Run(ctx context.Context, experimentPath, systemPath, revision string) (ael.EvidenceBundle, string, error) {
	root, err := filepath.Abs(l.Workspace)
	if err != nil {
		return ael.EvidenceBundle{}, "", err
	}
	experiment, err := ael.LoadExperiment(root, experimentPath)
	if err != nil {
		return ael.EvidenceBundle{}, "", err
	}
	system, err := ael.LoadSystem(root, systemPath)
	if err != nil {
		return ael.EvidenceBundle{}, "", err
	}
	if err := l.rewriteFirmware(root, &system); err != nil {
		return ael.EvidenceBundle{}, "", err
	}
	factories := map[ael.Backend]ael.AdapterFactory{}
	runtimePaths := map[string]string{}
	for _, component := range system.Components {
		backend := component.Backend
		factories[backend] = func(component ael.Component) (ael.Adapter, error) {
			adapter, runtime, err := l.adapter(root, component)
			if err == nil {
				runtimePaths[component.ID] = runtime
			}
			return adapter, err
		}
	}
	bundle, runErr := (ael.Scheduler{Factories: factories}).Run(ctx, experiment, system, revision)
	if bundle.RunID == "" {
		return bundle, "", runErr
	}
	sources := map[string]string{}
	for key, uri := range bundle.Artifacts {
		if !strings.HasPrefix(uri, "ael-runtime://") {
			continue
		}
		component, _, _ := strings.Cut(key, ".")
		runtime := runtimePaths[component]
		if runtime == "" {
			continue
		}
		candidate := filepath.Join(runtime, filepath.FromSlash(strings.TrimPrefix(uri, "ael-runtime://")))
		relative, err := filepath.Rel(runtime, candidate)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			sources[key] = candidate
		}
	}
	path, evidenceErr := (ael.EvidenceWriter{Workspace: root, ArtifactSources: sources}).Write(bundle)
	if evidenceErr != nil {
		return bundle, "", errors.Join(runErr, evidenceErr)
	}
	return bundle, path, runErr
}
func (l Lab) adapter(root string, component ael.Component) (ael.Adapter, string, error) {
	image := l.Images[component.Backend]
	if image == "" {
		return nil, "", fmt.Errorf("container image for %s is not configured", component.Backend)
	}
	worker, err := filepath.Abs(l.WorkerBinary)
	if err != nil {
		return nil, "", err
	}
	if info, err := os.Stat(worker); err != nil || !info.Mode().IsRegular() {
		return nil, "", errors.New("Go backend worker binary is unavailable")
	}
	runtimeRoot := l.RuntimeRoot
	if runtimeRoot == "" {
		runtimeRoot = filepath.Join(root, ".ael", "container-runs")
	}
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		return nil, "", err
	}
	runtime, err := os.MkdirTemp(runtimeRoot, "component-"+component.ID+"-")
	if err != nil {
		return nil, "", err
	}
	arguments := []string{"run", "--rm", "-i", "--network", "none", "--entrypoint", "/aether/ael-backend", "-e", "AEL_RUNTIME_ROOT=/runtime", "-v", worker + ":/aether/ael-backend:ro", "-v", filepath.Join(root, "benchmarks") + ":/workspace/benchmarks:ro", "-v", filepath.Join(root, "firmware") + ":/workspace/firmware:ro", "-v", runtime + ":/runtime"}
	if _, err := os.Stat(filepath.Join(root, ".ael", "firmware-builds")); err == nil {
		arguments = append(arguments, "-v", filepath.Join(root, ".ael", "firmware-builds")+":/workspace/.ael/firmware-builds:ro")
	}
	arguments = append(arguments, image, "--backend", string(component.Backend), "--workspace", "/workspace")
	adapter, err := ael.NewProcessAdapter(ael.ProcessConfig{Executable: "docker", Arguments: arguments, Directory: root, Timeout: duration(component.Properties, "timeout_s", 30*time.Minute)})
	return adapter, runtime, err
}

func (l Lab) Adapter(root string, component ael.Component) (ael.Adapter, string, error) {
	return l.adapter(root, component)
}

func (l Lab) RewriteFirmware(root string, system *ael.System) error {
	return l.rewriteFirmware(root, system)
}
func (l Lab) BuildFirmware(ctx context.Context, caseID int, variant string) error {
	if caseID < 4 || caseID > 24 || (variant != "faulty" && variant != "fixed") {
		return errors.New("invalid firmware mechanism build")
	}
	root, err := filepath.Abs(l.Workspace)
	if err != nil {
		return err
	}
	image := l.Images[ael.BackendZephyr]
	if image == "" {
		return errors.New("Zephyr image is not configured")
	}
	output := filepath.Join(root, ".ael", "firmware-builds", fmt.Sprintf("build-case%02d-%s", caseID, variant))
	cache := filepath.Join(root, ".ael", "zephyr-cache", fmt.Sprintf("case%02d-%s", caseID, variant))
	if err := os.MkdirAll(output, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return err
	}
	arguments := []string{"run", "--rm", "--network", "none", "--entrypoint", "west", "-v", filepath.Join(root, "firmware", "zephyr") + ":/src:ro", "-v", output + ":/out", "-v", cache + ":/cache"}
	buildArguments := []string{"build", "-p", "always", "-b", "stm32f4_disco", "/src", "-d", "/out", "--", "-DUSER_CACHE_DIR=/cache", "-DEXTRA_CONF_FILE=/src/conf/" + fmt.Sprintf("case%02d-%s.conf", caseID, variant)}
	if caseID == 17 {
		image = "ael-zephyr-mcuboot:local"
		arguments = append(arguments, "-v", filepath.Join(root, ".ael", "modules", "mcuboot")+":/opt/zephyrproject/bootloader/mcuboot:ro", "-v", filepath.Join(root, ".ael", "modules", "mbedtls")+":/opt/zephyrproject/modules/crypto/mbedtls:ro", "-v", filepath.Join(root, ".ael", "modules", "tf-psa-crypto")+":/opt/zephyrproject/modules/crypto/tf-psa-crypto:ro")
		buildArguments = append([]string{"build", "--sysbuild"}, buildArguments[1:]...)
		buildArguments = append(buildArguments, "-DDTC_OVERLAY_FILE=/src/overlays/case17.overlay", "-Dmcuboot_DTC_OVERLAY_FILE=/src/overlays/case17-mcuboot.overlay", "-Dmcuboot_EXTRA_CONF_FILE=/src/sysbuild/mcuboot.conf", "-DZEPHYR_EXTRA_MODULES=/opt/zephyrproject/bootloader/mcuboot;/opt/zephyrproject/modules/crypto/mbedtls")
	}
	arguments = append(arguments, image)
	arguments = append(arguments, buildArguments...)
	if caseID == 16 {
		arguments = append(arguments, "-DDTC_OVERLAY_FILE=/src/overlays/case16.overlay")
	}
	command := exec.CommandContext(ctx, "docker", arguments...)
	command.Dir = root
	outputLog, err := command.CombinedOutput()
	logPath := filepath.Join(output, "build.log")
	_ = os.WriteFile(logPath, outputLog, 0o600)
	if err != nil {
		return fmt.Errorf("Zephyr build case%02d-%s: %w: %s", caseID, variant, err, diagnostic(outputLog))
	}
	if caseID == 17 {
		candidate := filepath.Join(output, "candidate.signed.bin")
		signArguments := []string{"run", "--rm", "--network", "none", "--entrypoint", "python3", "-v", output + ":/out", "-v", filepath.Join(root, ".ael", "modules", "mcuboot") + ":/opt/zephyrproject/bootloader/mcuboot:ro", "ael-zephyr-mcuboot:local", "/opt/zephyrproject/bootloader/mcuboot/scripts/imgtool.py", "sign", "--version", "1.1.0", "--header-size", "0x200", "--slot-size", "131072", "--align", "1", "--key", "/opt/zephyrproject/bootloader/mcuboot/root-ed25519.pem", "/out/src/zephyr/zephyr.bin", "/out/candidate.signed.bin"}
		sign := exec.CommandContext(ctx, "docker", signArguments...)
		sign.Dir = root
		if signedLog, err := sign.CombinedOutput(); err != nil {
			return fmt.Errorf("sign MCUboot candidate: %w: %s", err, diagnostic(signedLog))
		}
		candidateHex := filepath.Join(output, "candidate.signed.hex")
		if err := binaryToIntelHex(candidate, candidateHex, 0x08040000); err != nil {
			return err
		}
		if err := l.FinalizeMCUbootArtifacts(variant); err != nil {
			return err
		}
	}
	artifact := filepath.Join(output, "zephyr", "zephyr.elf")
	if caseID == 17 {
		artifact = filepath.Join(output, "merged.hex")
	}
	if _, err := os.Stat(artifact); err != nil {
		return err
	}
	return nil
}

func (l Lab) FinalizeMCUbootArtifacts(variant string) error {
	if variant != "faulty" && variant != "fixed" {
		return errors.New("invalid MCUboot variant")
	}
	root, err := filepath.Abs(l.Workspace)
	if err != nil {
		return err
	}
	output := filepath.Join(root, ".ael", "firmware-builds", "build-case17-"+variant)
	erasedHex := filepath.Join(output, "flash-erased.hex")
	if err := fillIntelHex(erasedHex, 0x08000000, 0x100000, 0xff); err != nil {
		return err
	}
	return mergeIntelHex(filepath.Join(output, "merged.hex"), erasedHex, filepath.Join(output, "mcuboot", "zephyr", "zephyr.hex"), filepath.Join(output, "src", "zephyr", "zephyr.signed.hex"), filepath.Join(output, "candidate.signed.hex"))
}

func binaryToIntelHex(input, output string, base uint64) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	return bytesToIntelHex(data, output, base)
}
func fillIntelHex(output string, base uint64, size int, value byte) error {
	return bytesToIntelHex(bytes.Repeat([]byte{value}, size), output, base)
}
func bytesToIntelHex(data []byte, output string, base uint64) error {
	var lines []string
	currentUpper := uint64(^uint64(0))
	for offset := 0; offset < len(data); {
		absolute := base + uint64(offset)
		upper := absolute >> 16
		if upper != currentUpper {
			record := []byte{2, 0, 0, 4, byte(upper >> 8), byte(upper)}
			record = append(record, intelChecksum(record))
			lines = append(lines, ":"+strings.ToUpper(hex.EncodeToString(record)))
			currentUpper = upper
		}
		length := min(255, len(data)-offset)
		address := uint16(absolute & 0xffff)
		record := []byte{byte(length), byte(address >> 8), byte(address), 0}
		record = append(record, data[offset:offset+length]...)
		record = append(record, intelChecksum(record))
		lines = append(lines, ":"+strings.ToUpper(hex.EncodeToString(record)))
		offset += length
	}
	lines = append(lines, ":00000001FF")
	return os.WriteFile(output, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}
func intelChecksum(record []byte) byte {
	sum := byte(0)
	for _, value := range record {
		sum += value
	}
	return ^sum + 1
}

func mergeIntelHex(output string, inputs ...string) error {
	seen := map[uint64]byte{}
	minimum := ^uint64(0)
	maximum := uint64(0)
	for _, path := range inputs {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		upper := uint64(0)
		for _, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, ":") {
				return fmt.Errorf("invalid Intel HEX record in %s", path)
			}
			record, err := hex.DecodeString(strings.TrimPrefix(line, ":"))
			if err != nil || len(record) < 5 {
				return fmt.Errorf("invalid Intel HEX record in %s", path)
			}
			sum := byte(0)
			for _, value := range record {
				sum += value
			}
			if sum != 0 {
				return fmt.Errorf("Intel HEX checksum mismatch in %s", path)
			}
			length := int(record[0])
			if len(record) != length+5 {
				return fmt.Errorf("Intel HEX length mismatch in %s", path)
			}
			address := uint64(record[1])<<8 | uint64(record[2])
			kind := record[3]
			switch kind {
			case 0:
				for index, value := range record[4 : 4+length] {
					absolute := upper + address + uint64(index)
					if previous, ok := seen[absolute]; ok && previous != value && previous != 0xff {
						return fmt.Errorf("Intel HEX overlap at %#x", absolute)
					}
					seen[absolute] = value
					minimum = min(minimum, absolute)
					maximum = max(maximum, absolute)
				}
			case 1:
				continue
			case 4:
				if length != 2 {
					return fmt.Errorf("invalid extended address record")
				}
				upper = (uint64(record[4])<<8 | uint64(record[5])) << 16
			case 5:
				// Start-linear-address records are intentionally omitted. Renode
				// obtains the Cortex-M reset vector from the canonical flash image.
			default:
				return fmt.Errorf("unsupported Intel HEX record type %d in %s", kind, path)
			}
		}
	}
	if len(seen) == 0 || minimum > maximum {
		return errors.New("merged Intel HEX contains no data")
	}
	span := maximum - minimum + 1
	if span > 64*1024*1024 {
		return fmt.Errorf("merged Intel HEX span is too large: %d bytes", span)
	}
	canonical := bytes.Repeat([]byte{0xff}, int(span))
	for address, value := range seen {
		canonical[address-minimum] = value
	}
	return bytesToIntelHex(canonical, output, minimum)
}
func (l Lab) BuildRISCvFirmware(ctx context.Context) error {
	root, err := filepath.Abs(l.Workspace)
	if err != nil {
		return err
	}
	image := l.Images[ael.BackendZephyr]
	if image == "" {
		return errors.New("Zephyr image is not configured")
	}
	output := filepath.Join(root, ".ael", "firmware-builds", "build-hifive1")
	cache := filepath.Join(root, ".ael", "zephyr-cache", "hifive1")
	if err := os.MkdirAll(output, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return err
	}
	arguments := []string{"run", "--rm", "--network", "none", "--entrypoint", "west", "-v", filepath.Join(root, "firmware", "zephyr") + ":/src:ro", "-v", output + ":/out", "-v", cache + ":/cache", image, "build", "-p", "always", "-b", "hifive1_revb", "/src", "-d", "/out", "--", "-DUSER_CACHE_DIR=/cache", "-DEXTRA_CONF_FILE=/src/conf/case04-fixed.conf"}
	command := exec.CommandContext(ctx, "docker", arguments...)
	command.Dir = root
	log, err := command.CombinedOutput()
	_ = os.WriteFile(filepath.Join(output, "build.log"), log, 0o600)
	if err != nil {
		return fmt.Errorf("RISC-V Zephyr build: %w: %s", err, diagnostic(log))
	}
	if _, err := os.Stat(filepath.Join(output, "zephyr", "zephyr.elf")); err != nil {
		return err
	}
	return nil
}
func (l Lab) rewriteFirmware(root string, system *ael.System) error {
	for index := range system.Components {
		component := &system.Components[index]
		value, ok := component.Properties["firmware"].(string)
		if !ok || value == "" {
			continue
		}
		marker := "firmware/zephyr/"
		if !strings.HasPrefix(value, marker) {
			continue
		}
		relative := strings.TrimPrefix(value, marker)
		candidate := filepath.Join(root, ".ael", "firmware-builds", relative)
		if _, err := os.Stat(candidate); err != nil {
			return fmt.Errorf("firmware build is missing: %s", candidate)
		}
		component.Properties["firmware"] = filepath.ToSlash(filepath.Join(".ael", "firmware-builds", relative))
	}
	return nil
}
func duration(properties map[string]any, key string, fallback time.Duration) time.Duration {
	if value, ok := properties[key].(float64); ok && value > 0 {
		return time.Duration(value) * time.Second
	}
	return fallback
}
func diagnostic(data []byte) string {
	if len(data) < 4000 {
		return string(data)
	}
	return string(data[:2000]) + "\n...truncated...\n" + string(data[len(data)-2000:])
}
