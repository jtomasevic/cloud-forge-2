//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	internalSecret = "dev-internal-secret"
	echoService    = "cf-integration-echo"
	controlNS      = "cloudforge-control-plane"
	gatewayURL     = "http://127.0.0.1:18080"
)

var env *suiteEnv

type suiteEnv struct {
	repoRoot       string
	tmpDir         string
	kubeconfigPath string
	accountsURL    string
	provisionerURL string
	routerURL      string
	services       []*managedProcess
	httpClient     *http.Client
}

type managedProcess struct {
	name   string
	cancel context.CancelFunc
	cmd    *exec.Cmd
	out    lockedBuffer
	done   chan error
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type accountFixture struct {
	AccountID       string
	TenantID        string
	TenantSlug      string
	TenantCreatedAt string
	Email           string
	Password        string
	Token           string
}

type jobResponse struct {
	ID        string  `json:"id"`
	NetworkID string  `json:"networkId"`
	Type      string  `json:"type"`
	Status    string  `json:"status"`
	Error     *string `json:"errorMessage"`
}

type networkStatus struct {
	NetworkID     string  `json:"networkId"`
	TenantID      string  `json:"tenantId"`
	Status        string  `json:"status"`
	PodCIDR       string  `json:"podCIDR"`
	SvcCIDR       string  `json:"svcCIDR"`
	VClusterName  *string `json:"vclusterName"`
	FailureReason *string `json:"failureReason"`
}

func TestMain(m *testing.M) {
	if os.Getenv("CF_INTEGRATION") != "1" {
		log.Print("CF_INTEGRATION=1 is required; skipping full local integration suite")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var err error
	env, err = setupSuite(ctx)
	if err != nil {
		log.Printf("integration setup failed: %v", err)
		if env != nil {
			env.dumpLogs()
			env.shutdown()
		}
		os.Exit(1)
	}

	code := m.Run()
	env.shutdown()
	os.Exit(code)
}

func setupSuite(ctx context.Context) (*suiteEnv, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "cloudforge-integration-*")
	if err != nil {
		return nil, err
	}
	s := &suiteEnv{
		repoRoot:   root,
		tmpDir:     tmp,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}

	for _, tool := range []string{"go", "k3d", "kubectl", "vcluster"} {
		if _, err := exec.LookPath(tool); err != nil {
			return s, fmt.Errorf("required tool %q not found on PATH", tool)
		}
	}
	if err := s.writeKubeconfig(ctx); err != nil {
		return s, err
	}
	if err := s.kubectl(ctx, "cluster-info", "--request-timeout=8s"); err != nil {
		return s, fmt.Errorf("k3d Kubernetes API is not reachable: %w", err)
	}

	accountsAddr, err := freeAddr()
	if err != nil {
		return s, err
	}
	provisionerAddr, err := freeAddr()
	if err != nil {
		return s, err
	}
	routerAddr, err := freeAddr()
	if err != nil {
		return s, err
	}
	s.accountsURL = "http://" + accountsAddr
	s.provisionerURL = "http://" + provisionerAddr
	s.routerURL = "http://" + routerAddr

	baseEnv := []string{
		"SCYLLADB_HOSTS=localhost:9042",
		"SCYLLADB_KEYSPACE=cloudforge",
		"OPENBAO_ADDR=http://localhost:8200",
		"OPENBAO_TOKEN=dev-root-token",
		"CF_INTERNAL_SECRET=" + internalSecret,
	}

	if err := s.startProcess(ctx, "cf-accounts", filepath.Join(root, "services/cf-accounts"), append(baseEnv,
		"HTTP_ADDR="+accountsAddr,
		"KEYCLOAK_ADMIN_URL=http://localhost:8084/auth",
		"KEYCLOAK_REALM=cloudforge",
		"KEYCLOAK_ADMIN_REALM=master",
		"KEYCLOAK_ADMIN_CLIENT_ID=admin-cli",
		"KEYCLOAK_ADMIN_USERNAME=admin",
		"KEYCLOAK_ADMIN_PASSWORD=admin",
		"KEYCLOAK_LOGIN_CLIENT_ID=cf-console",
	)); err != nil {
		return s, err
	}
	if err := waitHTTP(ctx, s.httpClient, http.MethodGet, s.accountsURL+"/v1/accounts?limit=1", nil, http.StatusOK); err != nil {
		return s, fmt.Errorf("cf-accounts readiness: %w", err)
	}

	if err := s.startProcess(ctx, "cf-provisioner", filepath.Join(root, "services/cf-provisioner"), append(baseEnv,
		"HTTP_ADDR="+provisionerAddr,
		"HOST_KUBECONFIG="+s.kubeconfigPath,
		"KUBECONFIG="+s.kubeconfigPath,
		"CF_HTTPROUTE_NAMESPACE="+controlNS,
		"CF_GATEWAY_BACKEND_SERVICE="+echoService,
		"CF_GATEWAY_PARENT_NAME=cloudforge-gateway",
		"CF_GATEWAY_PARENT_NAMESPACE=envoy-gateway-system",
	)); err != nil {
		return s, err
	}
	if err := waitHTTP(ctx, s.httpClient, http.MethodGet, s.provisionerURL+"/v1/cidr/allocations", map[string]string{
		"X-CF-Internal-Secret": internalSecret,
	}, http.StatusOK); err != nil {
		return s, fmt.Errorf("cf-provisioner readiness: %w", err)
	}

	if err := s.startProcess(ctx, "cf-router", filepath.Join(root, "services/cf-router"), append(baseEnv,
		"HTTP_ADDR="+routerAddr,
		"SWAGGER_ADDR=disabled",
		"CF_ACCOUNTS_URL="+s.accountsURL,
		"CF_PROVISIONER_URL="+s.provisionerURL,
		"KEYCLOAK_JWKS_URL=http://localhost:8084/auth/realms/cloudforge/protocol/openid-connect/certs",
	)); err != nil {
		return s, err
	}
	if err := waitHTTP(ctx, s.httpClient, http.MethodGet, s.routerURL+"/ready", nil, http.StatusOK); err != nil {
		return s, fmt.Errorf("cf-router readiness: %w", err)
	}

	return s, nil
}

func TestAccountCreateAndLogin(t *testing.T) {
	acct := createAccount(t)
	t.Cleanup(func() { cleanupAccount(t, acct) })

	if acct.Token == "" {
		t.Fatal("expected login token")
	}

	status, _ := requestJSON(t, http.MethodPost, env.routerURL+"/v1/auth/login", nil, map[string]any{
		"email": acct.Email, "password": "wrong-password",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("invalid credentials status: got %d, want 401", status)
	}

	status, _ = requestJSON(t, http.MethodGet, env.routerURL+"/v1/accounts/"+acct.AccountID, nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("missing bearer token status: got %d, want 401", status)
	}

	status, _ = requestJSON(t, http.MethodGet, env.routerURL+"/v1/accounts/"+acct.AccountID, map[string]string{
		"Authorization": "Bearer invalid-token",
	}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("invalid bearer token status: got %d, want 401", status)
	}
}

func TestProvisionPrivateNetworkRouterGatewayAndCleanup(t *testing.T) {
	acct := createAccount(t)
	t.Cleanup(func() { cleanupAccount(t, acct) })

	other := createAccount(t)
	t.Cleanup(func() { cleanupAccount(t, other) })

	ensureEchoBackend(t)

	var networkID string
	t.Cleanup(func() {
		if networkID != "" {
			cleanupNetwork(t, acct.Token, networkID)
		}
	})

	status, body := requestJSON(t, http.MethodPost, env.routerURL+"/v1/networks", bearer(acct.Token), map[string]any{
		"region": "local",
	})
	if status != http.StatusAccepted {
		t.Fatalf("provision network status: got %d body %s", status, body)
	}
	var provisionJob jobResponse
	decodeBody(t, body, &provisionJob)
	networkID = provisionJob.NetworkID
	if networkID == "" {
		t.Fatal("provision job did not return networkId")
	}

	waitJobSucceeded(t, acct.Token, provisionJob.ID)
	st := waitNetworkStatus(t, acct.Token, networkID, "active")
	if st.TenantID != acct.TenantID {
		t.Fatalf("network tenant mismatch: got %s, want %s", st.TenantID, acct.TenantID)
	}
	if st.PodCIDR == "" || st.SvcCIDR == "" {
		t.Fatalf("expected allocated pod/service CIDRs, got %#v", st)
	}

	vcName := valueOr(st.VClusterName, vclusterName(networkID))
	assertKubernetesNetworkResources(t, vcName, networkID)

	status, body = requestJSON(t, http.MethodGet, env.routerURL+"/v1/networks/"+networkID, bearer(other.Token), nil)
	if status != http.StatusNotFound {
		t.Fatalf("wrong tenant network lookup status: got %d body %s, want 404", status, body)
	}

	host := "cf-it-" + shortID(networkID) + ".gateway.cloudforge.local"
	status, body = requestJSON(t, http.MethodPost, env.routerURL+"/v1/networks/"+networkID+"/gateway", bearer(acct.Token), map[string]any{
		"publicDNSName": host,
		"tlsEnabled":    false,
	})
	if status != http.StatusAccepted {
		t.Fatalf("provision gateway status: got %d body %s", status, body)
	}
	var gatewayJob jobResponse
	decodeBody(t, body, &gatewayJob)
	gatewayNetworkID := networkID
	t.Cleanup(func() { cleanupGateway(t, acct.Token, gatewayNetworkID) })
	waitJobSucceeded(t, acct.Token, gatewayJob.ID)
	waitGatewayHTTP(t, host)

	cleanupGateway(t, acct.Token, networkID)
	cleanupNetwork(t, acct.Token, networkID)
	networkID = ""

	assertNamespaceGone(t, vcName)
}

func (s *suiteEnv) startProcess(parent context.Context, name, dir string, extraEnv []string) error {
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	p := &managedProcess{name: name, cancel: cancel, cmd: cmd, done: make(chan error, 1)}
	cmd.Stdout = &p.out
	cmd.Stderr = &p.out
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start %s: %w", name, err)
	}
	go func() {
		p.done <- cmd.Wait()
	}()
	s.services = append(s.services, p)
	return nil
}

func (s *suiteEnv) shutdown() {
	for i := len(s.services) - 1; i >= 0; i-- {
		p := s.services[i]
		p.cancel()
		if p.cmd.Process != nil {
			_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
		}
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			if p.cmd.Process != nil {
				_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
			}
			<-p.done
		}
	}
	if s.tmpDir != "" {
		_ = os.RemoveAll(s.tmpDir)
	}
}

func (s *suiteEnv) dumpLogs() {
	for _, p := range s.services {
		log.Printf("=== %s logs ===\n%s", p.name, p.out.String())
	}
}

func (s *suiteEnv) writeKubeconfig(ctx context.Context) error {
	path := filepath.Join(s.tmpDir, "kubeconfig")
	cmd := exec.CommandContext(ctx, "k3d", "kubeconfig", "write", "cloudforge-dev", "--output", path, "--overwrite")
	if out, err := cmd.CombinedOutput(); err != nil {
		fallback := exec.CommandContext(ctx, "k3d", "kubeconfig", "get", "cloudforge-dev")
		raw, fallbackErr := fallback.Output()
		if fallbackErr != nil {
			return fmt.Errorf("write k3d kubeconfig: %w: %s", err, strings.TrimSpace(string(out)))
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			return err
		}
	}
	s.kubeconfigPath = path
	return nil
}

func (s *suiteEnv) kubectl(ctx context.Context, args ...string) error {
	all := append([]string{"--kubeconfig", s.kubeconfigPath}, args...)
	out, err := exec.CommandContext(ctx, "kubectl", all...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.work")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("could not find repository root containing go.work")
		}
		wd = parent
	}
}

func freeAddr() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	return ln.Addr().String(), nil
}

func waitHTTP(ctx context.Context, client *http.Client, method, url string, headers map[string]string, want int) error {
	deadline := time.Now().Add(90 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err == nil {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode == want {
				return nil
			}
			last = fmt.Sprintf("status %d body %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		} else {
			last = err.Error()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%s did not return %d: last %s", url, want, last)
}

func createAccount(t *testing.T) accountFixture {
	t.Helper()
	now := time.Now().UnixNano()
	email := fmt.Sprintf("cf-it-%d@example.com", now)
	password := "integration-password-1"
	status, body := requestJSON(t, http.MethodPost, env.routerURL+"/v1/accounts", nil, map[string]any{
		"email": email, "password": password,
	})
	if status != http.StatusCreated {
		t.Fatalf("create account status: got %d body %s", status, body)
	}
	var created struct {
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
		DefaultTenant struct {
			ID        string `json:"id"`
			Slug      string `json:"slug"`
			CreatedAt string `json:"createdAt"`
		} `json:"defaultTenant"`
	}
	decodeBody(t, body, &created)

	status, body = requestJSON(t, http.MethodPost, env.routerURL+"/v1/auth/login", nil, map[string]any{
		"email": email, "password": password,
	})
	if status != http.StatusOK {
		t.Fatalf("login status: got %d body %s", status, body)
	}
	var login struct {
		AccessToken string `json:"accessToken"`
	}
	decodeBody(t, body, &login)
	return accountFixture{
		AccountID:       created.Account.ID,
		TenantID:        created.DefaultTenant.ID,
		TenantSlug:      created.DefaultTenant.Slug,
		TenantCreatedAt: created.DefaultTenant.CreatedAt,
		Email:           email,
		Password:        password,
		Token:           login.AccessToken,
	}
}

func cleanupAccount(t *testing.T, acct accountFixture) {
	t.Helper()
	if acct.AccountID == "" || acct.Token == "" {
		return
	}
	status, body := requestJSON(t, http.MethodDelete, env.routerURL+"/v1/accounts/"+acct.AccountID, bearer(acct.Token), nil)
	if status != http.StatusNoContent && status != http.StatusNotFound && status != http.StatusUnauthorized {
		t.Logf("account cleanup status: got %d body %s", status, body)
	}
	purgeKeycloakUser(t, acct.AccountID)
	purgeAccountRows(t, acct)
}

func purgeKeycloakUser(t *testing.T, accountID string) {
	t.Helper()
	form := "grant_type=password&client_id=admin-cli&username=admin&password=admin"
	req, err := http.NewRequest(http.MethodPost, "http://localhost:8084/auth/realms/master/protocol/openid-connect/token", strings.NewReader(form))
	if err != nil {
		t.Logf("keycloak cleanup request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := env.httpClient.Do(req)
	if err != nil {
		t.Logf("keycloak cleanup token request: %v", err)
		return
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Logf("keycloak cleanup token status: got %d body %s", resp.StatusCode, raw)
		return
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &token); err != nil {
		t.Logf("keycloak cleanup token decode: %v", err)
		return
	}
	del, err := http.NewRequest(http.MethodDelete, "http://localhost:8084/auth/admin/realms/cloudforge/users/"+accountID, nil)
	if err != nil {
		t.Logf("keycloak cleanup delete request: %v", err)
		return
	}
	del.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err = env.httpClient.Do(del)
	if err != nil {
		t.Logf("keycloak cleanup delete: %v", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		t.Logf("keycloak cleanup delete status: got %d", resp.StatusCode)
	}
}

func purgeAccountRows(t *testing.T, acct accountFixture) {
	t.Helper()
	statements := []string{
		"DELETE FROM cloudforge.accounts WHERE id = " + acct.AccountID,
		"DELETE FROM cloudforge.accounts_by_email WHERE email = " + cqlQuote(acct.Email),
	}
	if acct.TenantID != "" {
		statements = append(statements, "DELETE FROM cloudforge.tenants WHERE id = "+acct.TenantID)
	}
	if acct.TenantSlug != "" {
		statements = append(statements, "DELETE FROM cloudforge.tenants_by_slug WHERE slug = "+cqlQuote(acct.TenantSlug))
	}
	if acct.AccountID != "" && acct.TenantID != "" && acct.TenantCreatedAt != "" {
		statements = append(statements, "DELETE FROM cloudforge.tenants_by_account WHERE account_id = "+acct.AccountID+" AND created_at = "+cqlTimestampLiteral(acct.TenantCreatedAt)+" AND tenant_id = "+acct.TenantID)
	}
	for _, stmt := range statements {
		if err := runCQL(stmt); err != nil {
			t.Logf("cql cleanup failed for %q: %v", stmt, err)
		}
	}
}

func runCQL(statement string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := exec.LookPath("cqlsh"); err == nil {
		out, err := exec.CommandContext(ctx, "cqlsh", "localhost", "9042", "-e", statement).CombinedOutput()
		if err == nil {
			return nil
		}
		return fmt.Errorf("cqlsh: %w: %s", err, strings.TrimSpace(string(out)))
	}
	out, err := exec.CommandContext(ctx, "docker", "compose", "-f", filepath.Join(env.repoRoot, "dev/docker-compose.yml"), "exec", "-T", "scylladb", "cqlsh", "scylladb", "9042", "-e", statement).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker cqlsh: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func cqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func cqlTimestampLiteral(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return cqlQuote(s)
	}
	return cqlQuote(t.UTC().Format("2006-01-02T15:04:05.000Z"))
}

func cleanupGateway(t *testing.T, token, networkID string) {
	t.Helper()
	status, body := requestJSON(t, http.MethodDelete, env.routerURL+"/v1/networks/"+networkID+"/gateway", bearer(token), nil)
	if status == http.StatusAccepted {
		var job jobResponse
		decodeBody(t, body, &job)
		waitJobSucceeded(t, token, job.ID)
		return
	}
	if status != http.StatusNotFound {
		t.Logf("gateway cleanup status: got %d body %s", status, body)
	}
}

func cleanupNetwork(t *testing.T, token, networkID string) {
	t.Helper()
	status, body := requestJSON(t, http.MethodDelete, env.routerURL+"/v1/networks/"+networkID, bearer(token), nil)
	if status == http.StatusAccepted {
		var job jobResponse
		decodeBody(t, body, &job)
		waitJobSucceeded(t, token, job.ID)
		return
	}
	if status != http.StatusNotFound {
		t.Logf("network cleanup status: got %d body %s", status, body)
	}
}

func waitJobSucceeded(t *testing.T, token, jobID string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Minute)
	var last jobResponse
	for time.Now().Before(deadline) {
		status, body := requestJSON(t, http.MethodGet, env.routerURL+"/v1/jobs/"+jobID, bearer(token), nil)
		if status != http.StatusOK {
			t.Fatalf("get job %s status: got %d body %s", jobID, status, body)
		}
		decodeBody(t, body, &last)
		switch last.Status {
		case "succeeded":
			return
		case "failed":
			t.Fatalf("job %s failed: %s", jobID, valueOr(last.Error, ""))
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timeout waiting for job %s to succeed; last status %#v", jobID, last)
}

func waitNetworkStatus(t *testing.T, token, networkID, want string) networkStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	var st networkStatus
	for time.Now().Before(deadline) {
		status, body := requestJSON(t, http.MethodGet, env.routerURL+"/v1/networks/"+networkID, bearer(token), nil)
		if status != http.StatusOK {
			t.Fatalf("get network status: got %d body %s", status, body)
		}
		decodeBody(t, body, &st)
		if st.Status == want {
			return st
		}
		if st.Status == "failed" {
			t.Fatalf("network %s failed: %s", networkID, valueOr(st.FailureReason, ""))
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("timeout waiting for network %s status %q; last %#v", networkID, want, st)
	return st
}

func ensureEchoBackend(t *testing.T) {
	t.Helper()
	manifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %[1]s
  namespace: %[2]s
  labels:
    app: %[1]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %[1]s
  template:
    metadata:
      labels:
        app: %[1]s
    spec:
      containers:
        - name: nginx
          image: nginx:1.25-alpine
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  selector:
    app: %[1]s
  ports:
    - name: http
      port: 80
      targetPort: 80
`, echoService, controlNS)
	kubectlStdin(t, "apply", "-f", "-", manifest)
	kubectl(t, "rollout", "status", "deployment/"+echoService, "-n", controlNS, "--timeout=180s")
}

func assertKubernetesNetworkResources(t *testing.T, vcName, networkID string) {
	t.Helper()
	kubectl(t, "get", "namespace", vcName)
	kubectl(t, "get", "statefulset", vcName, "-n", vcName)
	kubectl(t, "get", "ciliumnetworkpolicy", "default-deny-egress-"+networkID, "-n", vcName)
}

func assertNamespaceGone(t *testing.T, namespace string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		err := kubectlErr("get", "namespace", namespace)
		if err != nil && strings.Contains(err.Error(), "NotFound") {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("namespace %s still exists after cleanup", namespace)
}

func waitGatewayHTTP(t *testing.T, host string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, gatewayURL+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		resp, err := env.httpClient.Do(req)
		if err == nil {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			last = fmt.Sprintf("status %d body %s", resp.StatusCode, strings.TrimSpace(string(raw)))
			if resp.StatusCode == http.StatusOK && strings.Contains(string(raw), "nginx") {
				return
			}
		} else {
			last = err.Error()
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("gateway host %s did not route to echo backend: last %s", host, last)
}

func requestJSON(t *testing.T, method, url string, headers map[string]string, payload any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := env.httpClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func decodeBody(t *testing.T, raw []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode JSON %s: %v", raw, err)
	}
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func kubectl(t *testing.T, args ...string) {
	t.Helper()
	if err := kubectlErr(args...); err != nil {
		t.Fatal(err)
	}
}

func kubectlErr(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	all := append([]string{"--kubeconfig", env.kubeconfigPath}, args...)
	out, err := exec.CommandContext(ctx, "kubectl", all...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func kubectlStdin(t *testing.T, arg1, arg2, arg3, manifest string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	args := []string{"--kubeconfig", env.kubeconfigPath, arg1, arg2, arg3}
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl %s: %v: %s", strings.Join(args[2:], " "), err, strings.TrimSpace(string(out)))
	}
}

func vclusterName(networkID string) string {
	return "cf-" + shortID(networkID)
}

func shortID(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
			if b.Len() == 8 {
				return b.String()
			}
		}
	}
	return "net"
}

func valueOr(p *string, fallback string) string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return fallback
	}
	return strings.TrimSpace(*p)
}
