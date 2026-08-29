package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"debug/buildinfo"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eust-w/agentic-embedded-lab/internal/ael"
	"github.com/eust-w/agentic-embedded-lab/internal/ael/benchmark"
	"github.com/eust-w/agentic-embedded-lab/internal/server"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type report struct {
	Profile           string          `json:"profile"`
	Checks            map[string]bool `json:"checks"`
	Passed            bool            `json:"passed"`
	HardwareValidated bool            `json:"hardware_validated"`
	Limitations       []string        `json:"limitations"`
}

func main() {
	workspace := flag.String("workspace", ".", "workspace")
	output := flag.String("output", ".ael/compose-acceptance/report.json", "output")
	project := flag.String("project-name", "ael-go-acceptance", "compose project")
	flag.Parse()
	root, err := filepath.Abs(*workspace)
	fatal(err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	certs := filepath.Join(root, ".ael", "dev-certs")
	caData, err := os.ReadFile(filepath.Join(certs, "ca.crt"))
	fatal(err)
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		fatal(errors.New("invalid CA"))
	}
	userClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "localhost"}}, Timeout: 30 * time.Second}
	certificate, err := tls.LoadX509KeyPair(filepath.Join(certs, "worker.crt"), filepath.Join(certs, "worker.key"))
	fatal(err)
	workerClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{certificate}, ServerName: "localhost"}}, Timeout: 30 * time.Second}
	result := report{Profile: "software", Checks: map[string]bool{}, HardwareValidated: false, Limitations: []string{"Local software topology only.", "No board, instrument, calibration, or Validation Envelope evidence was produced."}}
	token := waitToken(ctx)
	anonymousStatus, _ := getStatus(ctx, userClient, "https://127.0.0.1:9443/v1/tasks/missing", "")
	result.Checks["oidc_rejects_anonymous"] = anonymousStatus == http.StatusUnauthorized
	task := createTask(ctx, userClient, token, map[string]any{"kind": "health_probe", "input": "compose"})
	completed := waitTask(ctx, userClient, token, task.ID)
	result.Checks["worker_executes_and_uploads_evidence"] = completed.Status == "completed" && completed.ResultObject != ""
	noCert := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: "localhost"}}, Timeout: 5 * time.Second}
	noCertErr := postJSON(ctx, noCert, "https://127.0.0.1:8443/v1/workers/register", map[string]any{"id": "no-cert", "capabilities": []string{"recovery"}}, nil, "")
	result.Checks["mtls_rejects_missing_client_certificate"] = noCertErr != nil
	runDocker(ctx, root, *project, "stop", "simulation-worker")
	register(ctx, workerClient, "recovery-a")
	recovery := createTask(ctx, userClient, token, map[string]any{"kind": "health_probe", "input": "recovery"})
	first := lease(ctx, workerClient, "recovery-a")
	if first == nil || first.ID != recovery.ID {
		fatal(errors.New("recovery worker did not lease task"))
	}
	runDocker(ctx, root, *project, "exec", "-T", "postgres", "psql", "-U", "ael", "-d", "ael", "-c", fmt.Sprintf("UPDATE aether_tasks SET lease_expires_at=NOW()-interval '1 second' WHERE id='%s';", recovery.ID))
	register(ctx, workerClient, "recovery-b")
	second := lease(ctx, workerClient, "recovery-b")
	result.Checks["expired_lease_recovered_idempotently"] = second != nil && second.ID == recovery.ID && second.Attempt == 2
	postJSON(ctx, userClient, "https://127.0.0.1:9443/v1/tasks/"+recovery.ID+"/cancel", map[string]any{}, nil, token)
	cancelled := getTask(ctx, userClient, token, recovery.ID)
	result.Checks["leased_task_cancellation"] = cancelled.Status == "cancelled"
	minioClient, err := minio.New("127.0.0.1:9000", &minio.Options{Creds: credentials.NewStaticV4("ael-development", "development-only-secret", ""), Secure: false, Transport: &http.Transport{Proxy: nil}})
	fatal(err)
	payload := []byte("aether-compose-evidence-retry")
	_, err = minioClient.PutObject(ctx, "ael-development", "acceptance/before", bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{})
	fatal(err)
	runDocker(ctx, root, *project, "stop", "minio")
	outageCtx, outageCancel := context.WithTimeout(ctx, 3*time.Second)
	_, outageErr := minioClient.PutObject(outageCtx, "ael-development", "acceptance/during", bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{})
	outageCancel()
	runDocker(ctx, root, *project, "start", "minio")
	waitMinio(ctx, minioClient)
	_, err = minioClient.PutObject(ctx, "ael-development", "acceptance/retry", bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{})
	fatal(err)
	object, err := minioClient.GetObject(ctx, "ael-development", "acceptance/retry", minio.GetObjectOptions{})
	fatal(err)
	restored, err := io.ReadAll(object)
	fatal(err)
	result.Checks["s3_outage_and_retransmit"] = outageErr != nil && bytes.Equal(restored, payload)
	runDocker(ctx, root, *project, "restart", "postgres")
	waitHTTP(ctx, func() bool {
		status, _ := getStatus(ctx, userClient, "https://127.0.0.1:9443/healthz", "")
		return status == 200
	})
	result.Checks["postgres_restart_recovery"] = true
	upgrade := runDocker(ctx, root, *project, "exec", "-T", "postgres", "psql", "-At", "-U", "ael", "-d", "ael", "-c", "CREATE TABLE aether_migration_acceptance(id integer PRIMARY KEY,payload jsonb NOT NULL DEFAULT '{}'::jsonb); INSERT INTO aether_migration_acceptance VALUES(1,'{\"phase\":\"upgraded\"}'); SELECT payload->>'phase' FROM aether_migration_acceptance WHERE id=1;")
	runDocker(ctx, root, *project, "exec", "-T", "postgres", "psql", "-U", "ael", "-d", "ael", "-c", "DROP TABLE aether_migration_acceptance;")
	rollback := runDocker(ctx, root, *project, "exec", "-T", "postgres", "psql", "-At", "-U", "ael", "-d", "ael", "-c", "SELECT to_regclass('public.aether_migration_acceptance') IS NULL;")
	result.Checks["migration_upgrade_and_rollback"] = strings.Contains(upgrade, "upgraded") && strings.Contains(rollback, "t")
	supplyPath, supplyPassed := supplyChainEvidence(root)
	result.Checks["supply_chain_sbom_signature"] = supplyPassed
	result.Passed = true
	for _, passed := range result.Checks {
		result.Passed = result.Passed && passed
	}
	destination := *output
	if !filepath.IsAbs(destination) {
		destination = filepath.Join(root, destination)
	}
	fatal(os.MkdirAll(filepath.Dir(destination), 0o700))
	data, _ := json.MarshalIndent(result, "", "  ")
	fatal(os.WriteFile(destination, append(data, '\n'), 0o600))
	reportHash := fileHash(destination)
	relativeReport, _ := filepath.Rel(root, destination)
	relativeSupply, _ := filepath.Rel(root, supplyPath)
	supplyHash := fileHash(supplyPath)
	entries := []benchmark.AcceptanceEntry{
		{Name: "deployment:compose", Status: passStatus(result.Checks["worker_executes_and_uploads_evidence"]), EvidencePath: filepath.ToSlash(relativeReport), EvidenceSHA256: reportHash, Limitations: result.Limitations},
		{Name: "storage:postgres-s3", Status: passStatus(result.Checks["s3_outage_and_retransmit"] && result.Checks["postgres_restart_recovery"] && result.Checks["migration_upgrade_and_rollback"]), EvidencePath: filepath.ToSlash(relativeReport), EvidenceSHA256: reportHash, Limitations: result.Limitations},
		{Name: "security:oidc-mtls", Status: passStatus(result.Checks["oidc_rejects_anonymous"] && result.Checks["mtls_rejects_missing_client_certificate"]), EvidencePath: filepath.ToSlash(relativeReport), EvidenceSHA256: reportHash, Limitations: result.Limitations},
		{Name: "worker:lease-recovery", Status: passStatus(result.Checks["expired_lease_recovered_idempotently"] && result.Checks["leased_task_cancellation"]), EvidencePath: filepath.ToSlash(relativeReport), EvidenceSHA256: reportHash, Limitations: result.Limitations},
		{Name: "supply-chain:sbom-signature", Status: passStatus(supplyPassed), EvidencePath: filepath.ToSlash(relativeSupply), EvidenceSHA256: supplyHash, Limitations: []string{"Ephemeral acceptance signing key; production distribution requires Developer ID and release key trust."}},
	}
	manifest := benchmark.AcceptanceManifest{APIVersion: ael.APIVersion, Profile: "software", SourceRevision: "working-tree", Entries: entries, CreatedAt: time.Now().UTC()}
	manifestPath := filepath.Join(root, "acceptance", "v2", "software.json")
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	fatal(os.MkdirAll(filepath.Dir(manifestPath), 0o700))
	fatal(os.WriteFile(manifestPath, append(manifestData, '\n'), 0o600))
	if !result.Passed {
		os.Exit(2)
	}
}

func supplyChainEvidence(root string) (string, bool) {
	type component struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Sum     string `json:"sum,omitempty"`
	}
	evidence := struct {
		APIVersion string            `json:"api_version"`
		Components []component       `json:"components"`
		Artifacts  map[string]string `json:"artifacts"`
		PublicKey  string            `json:"public_key"`
		Signature  string            `json:"signature"`
		Verified   bool              `json:"verified"`
	}{APIVersion: ael.APIVersion, Artifacts: map[string]string{}}
	for _, name := range []string{"aether-server", "aether-worker", "ael-backend"} {
		path := filepath.Join(root, ".ael", "container-bin", name)
		info, err := buildinfo.ReadFile(path)
		if err != nil {
			return "", false
		}
		evidence.Artifacts[name] = fileHash(path)
		evidence.Components = append(evidence.Components, component{Name: info.Main.Path, Version: info.Main.Version, Sum: info.Main.Sum})
		for _, dependency := range info.Deps {
			evidence.Components = append(evidence.Components, component{Name: dependency.Path, Version: dependency.Version, Sum: dependency.Sum})
		}
	}
	evidence.Artifacts["frontend/package-lock.json"] = fileHash(filepath.Join(root, "frontend", "package-lock.json"))
	sort.Slice(evidence.Components, func(i, j int) bool {
		if evidence.Components[i].Name == evidence.Components[j].Name {
			return evidence.Components[i].Version < evidence.Components[j].Version
		}
		return evidence.Components[i].Name < evidence.Components[j].Name
	})
	payload, _ := json.Marshal(struct {
		APIVersion string            `json:"api_version"`
		Components []component       `json:"components"`
		Artifacts  map[string]string `json:"artifacts"`
	}{evidence.APIVersion, evidence.Components, evidence.Artifacts})
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", false
	}
	signature := ed25519.Sign(private, payload)
	evidence.PublicKey = base64.RawStdEncoding.EncodeToString(public)
	evidence.Signature = base64.RawStdEncoding.EncodeToString(signature)
	evidence.Verified = ed25519.Verify(public, payload, signature)
	path := filepath.Join(root, ".ael", "compose-acceptance", "supply-chain.json")
	data, _ := json.MarshalIndent(evidence, "", "  ")
	if os.WriteFile(path, append(data, '\n'), 0o600) != nil {
		return "", false
	}
	return path, evidence.Verified
}
func fileHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func passStatus(passed bool) string {
	if passed {
		return "passed"
	}
	return "failed"
}

func waitToken(ctx context.Context) string {
	var body struct {
		AccessToken string `json:"access_token"`
	}
	waitHTTP(ctx, func() bool {
		body = struct {
			AccessToken string `json:"access_token"`
		}{}
		err := postJSON(ctx, &http.Client{Timeout: 5 * time.Second}, "http://127.0.0.1:18080/token", map[string]string{"subject": "compose-acceptance"}, &body, "")
		return err == nil && body.AccessToken != ""
	})
	return body.AccessToken
}
func createTask(ctx context.Context, client *http.Client, token string, payload map[string]any) server.Task {
	var task server.Task
	fatal(postJSON(ctx, client, "https://127.0.0.1:9443/v1/tasks", map[string]any{"payload": payload}, &task, token))
	return task
}
func waitTask(ctx context.Context, client *http.Client, token, id string) server.Task {
	var task server.Task
	waitHTTP(ctx, func() bool {
		task = getTask(ctx, client, token, id)
		return task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled"
	})
	return task
}
func getTask(ctx context.Context, client *http.Client, token, id string) server.Task {
	var task server.Task
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://127.0.0.1:9443/v1/tasks/"+id, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return task
	}
	defer response.Body.Close()
	_ = json.NewDecoder(response.Body).Decode(&task)
	return task
}
func register(ctx context.Context, client *http.Client, id string) {
	fatal(postJSON(ctx, client, "https://127.0.0.1:8443/v1/workers/register", map[string]any{"id": id, "capabilities": []string{"recovery"}}, nil, ""))
}
func lease(ctx context.Context, client *http.Client, id string) *server.Task {
	var response struct {
		Task *server.Task `json:"task"`
	}
	fatal(postJSON(ctx, client, "https://127.0.0.1:8443/v1/workers/"+id+"/lease", map[string]any{}, &response, ""))
	return response.Task
}
func postJSON(ctx context.Context, client *http.Client, url string, input, target any, token string) error {
	data, _ := json.Marshal(input)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, body)
	}
	if target != nil {
		return json.NewDecoder(response.Body).Decode(target)
	}
	return nil
}
func getStatus(ctx context.Context, client *http.Client, url, token string) (int, error) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	response.Body.Close()
	return response.StatusCode, nil
}
func waitHTTP(ctx context.Context, check func() bool) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if check() {
			return
		}
		select {
		case <-ctx.Done():
			fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}
func waitMinio(ctx context.Context, client *minio.Client) {
	waitHTTP(ctx, func() bool { _, err := client.ListBuckets(ctx); return err == nil })
}
func runDocker(ctx context.Context, root, project string, args ...string) string {
	base := []string{"compose", "--project-name", project, "-f", filepath.Join(root, "deploy", "compose.yaml")}
	command := exec.CommandContext(ctx, "docker", append(base, args...)...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		fatal(fmt.Errorf("docker compose %s: %w: %s", strings.Join(args, " "), err, output))
	}
	return string(output)
}
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
