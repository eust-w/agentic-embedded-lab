package server

import (
	"bytes"
	"context"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	capability "github.com/eust-w/agentic-embedded-lab/internal/acceptance"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Workspace          string
	SourceRevision     string
	DatabaseURL        string
	S3Endpoint         string
	S3Bucket           string
	S3AccessKey        string
	S3SecretKey        string
	OIDCIssuer         string
	OIDCAudience       string
	OIDCJWKSURL        string
	WorkerFingerprints map[string]bool
}

type ControlPlane struct {
	config  Config
	db      *sql.DB
	objects *minio.Client
	auth    *OIDCValidator
}

func Open(ctx context.Context, config Config) (*ControlPlane, error) {
	if config.DatabaseURL == "" || config.S3Endpoint == "" || config.S3Bucket == "" {
		return nil, errors.New("database and S3 configuration are required")
	}
	db, err := sql.Open("pgx", strings.Replace(config.DatabaseURL, "postgresql+psycopg://", "postgresql://", 1))
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	endpoint := strings.TrimPrefix(strings.TrimPrefix(config.S3Endpoint, "http://"), "https://")
	secure := strings.HasPrefix(config.S3Endpoint, "https://")
	objects, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(config.S3AccessKey, config.S3SecretKey, ""), Secure: secure})
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := objects.MakeBucket(ctx, config.S3Bucket, minio.MakeBucketOptions{}); err != nil {
		exists, _ := objects.BucketExists(ctx, config.S3Bucket)
		if !exists {
			db.Close()
			return nil, err
		}
	}
	return &ControlPlane{config: config, db: db, objects: objects, auth: &OIDCValidator{Issuer: config.OIDCIssuer, Audience: config.OIDCAudience, JWKSURL: config.OIDCJWKSURL, HTTP: &http.Client{Timeout: 10 * time.Second}}}, nil
}
func (c *ControlPlane) Close() error { return c.db.Close() }
func (c *ControlPlane) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", c.health)
	mux.HandleFunc("POST /v1/tasks", c.user(c.createTask))
	mux.HandleFunc("GET /v1/tasks/{id}", c.user(c.getTask))
	mux.HandleFunc("POST /v1/tasks/{id}/cancel", c.user(c.cancelUserTask))
	mux.HandleFunc("GET /v1/capabilities", c.user(c.listCapabilities))
	mux.HandleFunc("GET /v1/capabilities/{id}", c.user(c.getCapability))
	mux.HandleFunc("POST /v1/acceptance/runs", c.user(c.createAcceptanceRun))
	mux.HandleFunc("GET /v1/acceptance/runs/{id}", c.user(c.getTask))
	mux.HandleFunc("POST /v1/workers/register", c.worker(c.registerWorker))
	mux.HandleFunc("POST /v1/workers/{worker}/lease", c.worker(c.leaseTask))
	mux.HandleFunc("POST /v1/workers/{worker}/heartbeat", c.worker(c.heartbeat))
	mux.HandleFunc("POST /v1/workers/{worker}/tasks/{task}/complete", c.worker(c.completeTask))
	mux.HandleFunc("POST /v1/workers/{worker}/tasks/{task}/cancelled", c.worker(c.cancelTask))
	return requestLimit(mux)
}

func (c *ControlPlane) listCapabilities(w http.ResponseWriter, r *http.Request) {
	values, err := capability.List(c.config.Workspace, c.config.SourceRevision)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"api_version": capability.APIVersion, "capabilities": values})
}
func (c *ControlPlane) getCapability(w http.ResponseWriter, r *http.Request) {
	value, err := capability.Inspect(c.config.Workspace, c.config.SourceRevision, r.PathValue("id"))
	if err != nil {
		writeError(w, 404, err)
		return
	}
	writeJSON(w, 200, value)
}
func (c *ControlPlane) createAcceptanceRun(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Profile string `json:"profile"`
	}
	if decodeJSON(w, r, &request) != nil {
		return
	}
	allowed := map[string]bool{"desktop": true, "agent": true, "simulation": true, "software": true, "development-package": true}
	if !allowed[request.Profile] {
		writeError(w, 400, errors.New("unsupported acceptance profile"))
		return
	}
	payload := map[string]any{"kind": "acceptance", "profile": request.Profile, "source_revision": c.config.SourceRevision}
	now := time.Now().UTC()
	task := Task{ID: uuid.NewString(), Status: "queued", Payload: payload, CreatedAt: now, UpdatedAt: now}
	encoded, _ := json.Marshal(payload)
	if _, err := c.db.ExecContext(r.Context(), `INSERT INTO aether_tasks(id,status,payload,created_at,updated_at)VALUES($1,$2,$3,$4,$4)`, task.ID, task.Status, encoded, now); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 202, task)
}

func migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS aether_workers(id TEXT PRIMARY KEY,capabilities JSONB NOT NULL,last_seen TIMESTAMPTZ NOT NULL,created_at TIMESTAMPTZ NOT NULL);CREATE TABLE IF NOT EXISTS aether_tasks(id UUID PRIMARY KEY,status TEXT NOT NULL,payload JSONB NOT NULL,lease_owner TEXT,lease_expires_at TIMESTAMPTZ,attempt INTEGER NOT NULL DEFAULT 0,result_object TEXT,error TEXT,created_at TIMESTAMPTZ NOT NULL,updated_at TIMESTAMPTZ NOT NULL);CREATE INDEX IF NOT EXISTS aether_tasks_queue ON aether_tasks(status,created_at);`)
	return err
}
func (c *ControlPlane) health(w http.ResponseWriter, r *http.Request) {
	var one int
	if err := c.db.QueryRowContext(r.Context(), "SELECT 1").Scan(&one); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "storage": "postgres+s3"})
}

type Task struct {
	ID           string         `json:"id"`
	Status       string         `json:"status"`
	Payload      map[string]any `json:"payload"`
	LeaseOwner   string         `json:"lease_owner,omitempty"`
	LeaseExpires *time.Time     `json:"lease_expires_at,omitempty"`
	Attempt      int            `json:"attempt"`
	ResultObject string         `json:"result_object,omitempty"`
	Error        string         `json:"error,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

func (c *ControlPlane) createTask(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Payload map[string]any `json:"payload"`
	}
	if decodeJSON(w, r, &request) != nil {
		return
	}
	if len(request.Payload) == 0 {
		writeError(w, 400, errors.New("task payload is required"))
		return
	}
	now := time.Now().UTC()
	task := Task{ID: uuid.NewString(), Status: "queued", Payload: request.Payload, CreatedAt: now, UpdatedAt: now}
	payload, _ := json.Marshal(task.Payload)
	_, err := c.db.ExecContext(r.Context(), `INSERT INTO aether_tasks(id,status,payload,created_at,updated_at)VALUES($1,$2,$3,$4,$4)`, task.ID, task.Status, payload, now)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 202, task)
}
func (c *ControlPlane) getTask(w http.ResponseWriter, r *http.Request) {
	task, err := c.readTask(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, 404, err)
		return
	}
	writeJSON(w, 200, task)
}
func (c *ControlPlane) cancelUserTask(w http.ResponseWriter, r *http.Request) {
	result, err := c.db.ExecContext(r.Context(), `UPDATE aether_tasks SET status='cancelled',lease_expires_at=NULL,updated_at=NOW() WHERE id=$1 AND status IN ('queued','running')`, r.PathValue("id"))
	if err != nil {
		writeError(w, 500, err)
		return
	}
	changed, _ := result.RowsAffected()
	writeJSON(w, 200, map[string]bool{"cancelled": changed == 1})
}
func (c *ControlPlane) registerWorker(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID           string   `json:"id"`
		Capabilities []string `json:"capabilities"`
	}
	if decodeJSON(w, r, &request) != nil {
		return
	}
	if request.ID == "" {
		writeError(w, 400, errors.New("worker id is required"))
		return
	}
	payload, _ := json.Marshal(request.Capabilities)
	now := time.Now().UTC()
	_, err := c.db.ExecContext(r.Context(), `INSERT INTO aether_workers(id,capabilities,last_seen,created_at)VALUES($1,$2,$3,$3) ON CONFLICT(id)DO UPDATE SET capabilities=excluded.capabilities,last_seen=excluded.last_seen`, request.ID, payload, now)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"registered": true, "lease_seconds": 120})
}
func (c *ControlPlane) leaseTask(w http.ResponseWriter, r *http.Request) {
	worker := r.PathValue("worker")
	tx, err := c.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer tx.Rollback()
	var task Task
	var payload []byte
	err = tx.QueryRowContext(r.Context(), `SELECT id,payload,attempt,created_at,updated_at FROM aether_tasks WHERE status='queued' OR (status='running' AND lease_expires_at<NOW()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&task.ID, &payload, &task.Attempt, &task.CreatedAt, &task.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 200, map[string]any{"task": nil})
		return
	}
	if err != nil {
		writeError(w, 500, err)
		return
	}
	expires := time.Now().UTC().Add(2 * time.Minute)
	task.Status = "running"
	task.LeaseOwner = worker
	task.LeaseExpires = &expires
	task.Attempt++
	_ = json.Unmarshal(payload, &task.Payload)
	if _, err = tx.ExecContext(r.Context(), `UPDATE aether_tasks SET status='running',lease_owner=$1,lease_expires_at=$2,attempt=attempt+1,updated_at=NOW() WHERE id=$3`, worker, expires, task.ID); err != nil {
		writeError(w, 500, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"task": task})
}
func (c *ControlPlane) heartbeat(w http.ResponseWriter, r *http.Request) {
	worker := r.PathValue("worker")
	var request struct {
		TaskID string `json:"task_id"`
	}
	if decodeJSON(w, r, &request) != nil {
		return
	}
	result, err := c.db.ExecContext(r.Context(), `UPDATE aether_tasks SET lease_expires_at=NOW()+INTERVAL '2 minutes',updated_at=NOW() WHERE id=$1 AND lease_owner=$2 AND status='running'`, request.TaskID, worker)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		writeError(w, 409, errors.New("task lease is not owned by worker"))
		return
	}
	writeJSON(w, 200, map[string]bool{"renewed": true})
}
func (c *ControlPlane) completeTask(w http.ResponseWriter, r *http.Request) {
	worker, taskID := r.PathValue("worker"), r.PathValue("task")
	var request struct {
		Result map[string]any `json:"result"`
	}
	if decodeJSON(w, r, &request) != nil {
		return
	}
	payload, _ := json.Marshal(request.Result)
	objectName := "evidence/" + taskID + ".json"
	if _, err := c.objects.PutObject(r.Context(), c.config.S3Bucket, objectName, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{ContentType: "application/json"}); err != nil {
		writeError(w, 503, err)
		return
	}
	result, err := c.db.ExecContext(r.Context(), `UPDATE aether_tasks SET status='completed',result_object=$1,lease_expires_at=NULL,updated_at=NOW() WHERE id=$2 AND lease_owner=$3 AND status='running'`, objectName, taskID, worker)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		writeError(w, 409, errors.New("task lease is not owned by worker"))
		return
	}
	writeJSON(w, 200, map[string]any{"completed": true, "object": objectName})
}
func (c *ControlPlane) cancelTask(w http.ResponseWriter, r *http.Request) {
	worker, taskID := r.PathValue("worker"), r.PathValue("task")
	result, err := c.db.ExecContext(r.Context(), `UPDATE aether_tasks SET status='cancelled',lease_expires_at=NULL,updated_at=NOW() WHERE id=$1 AND lease_owner=$2 AND status='running'`, taskID, worker)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	changed, _ := result.RowsAffected()
	writeJSON(w, 200, map[string]bool{"cancelled": changed == 1})
}
func (c *ControlPlane) readTask(ctx context.Context, id string) (Task, error) {
	var task Task
	var payload []byte
	err := c.db.QueryRowContext(ctx, `SELECT id,status,payload,COALESCE(lease_owner,''),lease_expires_at,attempt,COALESCE(result_object,''),COALESCE(error,''),created_at,updated_at FROM aether_tasks WHERE id=$1`, id).Scan(&task.ID, &task.Status, &payload, &task.LeaseOwner, &task.LeaseExpires, &task.Attempt, &task.ResultObject, &task.Error, &task.CreatedAt, &task.UpdatedAt)
	_ = json.Unmarshal(payload, &task.Payload)
	return task, err
}

func (c *ControlPlane) worker(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Aether-Ingress") != "worker" {
			writeError(w, http.StatusUnauthorized, errors.New("worker route requires the mTLS ingress"))
			return
		}
		fingerprint := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Client-Cert-SHA256")))
		if !c.config.WorkerFingerprints[fingerprint] {
			writeError(w, 401, errors.New("untrusted worker certificate"))
			return
		}
		next(w, r)
	}
}
func (c *ControlPlane) user(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" {
			writeError(w, 401, errors.New("bearer token required"))
			return
		}
		if _, err := c.auth.Validate(r.Context(), token); err != nil {
			writeError(w, 401, err)
			return
		}
		next(w, r)
	}
}

type OIDCValidator struct {
	Issuer, Audience, JWKSURL string
	HTTP                      *http.Client
	mu                        sync.Mutex
	keys                      map[string]*rsa.PublicKey
}

func (v *OIDCValidator) Validate(ctx context.Context, raw string) (jwt.MapClaims, error) {
	if v.Issuer == "" || v.Audience == "" || v.JWKSURL == "" {
		return nil, errors.New("OIDC is not configured")
	}
	token, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != "RS256" {
			return nil, errors.New("only RS256 is accepted")
		}
		kid, _ := token.Header["kid"].(string)
		return v.key(ctx, kid)
	}, jwt.WithIssuer(v.Issuer), jwt.WithAudience(v.Audience), jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{"RS256"}))
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid OIDC token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid OIDC claims")
	}
	return claims, nil
}
func (v *OIDCValidator) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if key := v.keys[kid]; key != nil {
		return key, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.JWKSURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := v.HTTP.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("JWKS HTTP %d", response.StatusCode)
	}
	var body struct {
		Keys []struct{ Kty, Kid, N, E string }
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		return nil, err
	}
	v.keys = map[string]*rsa.PublicKey{}
	for _, jwk := range body.Keys {
		nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
		if err != nil {
			continue
		}
		exponent := 0
		for _, value := range eBytes {
			exponent = exponent<<8 + int(value)
		}
		v.keys[jwk.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: exponent}
	}
	key := v.keys[kid]
	if key == nil {
		return nil, errors.New("OIDC signing key is unknown")
	}
	return key, nil
}

func ConfigFromEnv() Config {
	fingerprints := map[string]bool{}
	for _, item := range strings.Split(os.Getenv("AEL_WORKER_FINGERPRINTS"), ",") {
		if value := strings.ToLower(strings.TrimSpace(item)); value != "" {
			fingerprints[value] = true
		}
	}
	workspace := os.Getenv("AEL_WORKSPACE")
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	revision := os.Getenv("AEL_SOURCE_REVISION")
	if revision == "" {
		command := exec.Command("git", "-C", workspace, "rev-parse", "HEAD")
		if data, err := command.Output(); err == nil {
			revision = strings.TrimSpace(string(data))
		}
	}
	return Config{Workspace: workspace, SourceRevision: revision, DatabaseURL: os.Getenv("AEL_DATABASE_URL"), S3Endpoint: os.Getenv("AEL_S3_ENDPOINT"), S3Bucket: os.Getenv("AEL_S3_BUCKET"), S3AccessKey: os.Getenv("AWS_ACCESS_KEY_ID"), S3SecretKey: os.Getenv("AWS_SECRET_ACCESS_KEY"), OIDCIssuer: os.Getenv("AEL_OIDC_ISSUER"), OIDCAudience: os.Getenv("AEL_OIDC_AUDIENCE"), OIDCJWKSURL: os.Getenv("AEL_OIDC_JWKS_URL"), WorkerFingerprints: fingerprints}
}
func requestLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		next.ServeHTTP(w, r)
	})
}
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, 400, err)
		return err
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
