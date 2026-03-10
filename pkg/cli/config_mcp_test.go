package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigDefaultEnv verifies default_env is parsed and merged.
func TestLoadConfigDefaultEnv(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`default_env: staging
environments:
  staging:
    namespace: staging-ns
`), 0o644)

	cfg := LoadConfig()
	if cfg.DefaultEnv != "staging" {
		t.Errorf("expected default_env=staging, got %q", cfg.DefaultEnv)
	}
}

// TestMergeConfigDefaultEnv verifies project config overrides user's default_env.
func TestMergeConfigDefaultEnv(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	// User config: default_env=dev
	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`default_env: dev
environments:
  dev:
    namespace: dev-ns
`), 0o644)

	// Project config: overrides default_env=staging
	projDir := filepath.Join(tmpDir, ".tentacular")
	_ = os.MkdirAll(projDir, 0o755)
	_ = os.WriteFile(filepath.Join(projDir, "config.yaml"), []byte(`default_env: staging
environments:
  staging:
    namespace: staging-ns
`), 0o644)

	cfg := LoadConfig()
	if cfg.DefaultEnv != "staging" {
		t.Errorf("expected project default_env=staging to override user, got %q", cfg.DefaultEnv)
	}
}

// TestLoadConfigPerEnvMCPEndpoint verifies mcp_endpoint is parsed from
// per-environment config.
func TestLoadConfigPerEnvMCPEndpoint(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`environments:
  prod:
    namespace: prod-ns
    mcp_endpoint: http://prod-mcp.tentacular-system.svc.cluster.local:8080
    mcp_token_path: ~/.tentacular/prod-token
`), 0o644)

	cfg := LoadConfig()
	prod, ok := cfg.Environments["prod"]
	if !ok {
		t.Fatal("expected prod environment")
	}
	if prod.MCPEndpoint != "http://prod-mcp.tentacular-system.svc.cluster.local:8080" {
		t.Errorf("expected prod mcp_endpoint, got %q", prod.MCPEndpoint)
	}
	if prod.MCPTokenPath != "~/.tentacular/prod-token" {
		t.Errorf("expected prod mcp_token_path, got %q", prod.MCPTokenPath)
	}
}

// TestEnvironmentMCPEndpointOmittedWhenEmpty verifies mcp_endpoint is
// omitted when not set (omitempty).
func TestEnvironmentMCPEndpointOmittedWhenEmpty(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`environments:
  dev:
    namespace: dev-ns
`), 0o644)

	env, err := LoadEnvironment("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.MCPEndpoint != "" {
		t.Errorf("expected empty mcp_endpoint when not configured, got %q", env.MCPEndpoint)
	}
	if env.MCPTokenPath != "" {
		t.Errorf("expected empty mcp_token_path when not configured, got %q", env.MCPTokenPath)
	}
}

// TestLoadConfigMultipleEnvsWithMCP verifies multiple environments can each
// have their own mcp_endpoint.
func TestLoadConfigMultipleEnvsWithMCP(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`default_env: dev
environments:
  dev:
    namespace: dev-ns
    mcp_endpoint: http://dev-mcp:8080
  staging:
    namespace: staging-ns
    mcp_endpoint: http://staging-mcp:8080
  prod:
    namespace: prod-ns
    mcp_endpoint: http://prod-mcp:8080
    mcp_token_path: /etc/tentacular/prod-token
`), 0o644)

	cfg := LoadConfig()
	if cfg.DefaultEnv != "dev" {
		t.Errorf("expected default_env=dev, got %q", cfg.DefaultEnv)
	}
	if len(cfg.Environments) != 3 {
		t.Errorf("expected 3 environments, got %d", len(cfg.Environments))
	}

	dev := cfg.Environments["dev"]
	if dev.MCPEndpoint != "http://dev-mcp:8080" {
		t.Errorf("expected dev mcp_endpoint http://dev-mcp:8080, got %q", dev.MCPEndpoint)
	}

	prod := cfg.Environments["prod"]
	if prod.MCPTokenPath != "/etc/tentacular/prod-token" {
		t.Errorf("expected prod mcp_token_path /etc/tentacular/prod-token, got %q", prod.MCPTokenPath)
	}
}

// TestResolveEnvironmentUsesDefaultEnv verifies that ResolveEnvironment
// picks up default_env when no explicit name is given.
func TestResolveEnvironmentUsesDefaultEnv(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	// Ensure TENTACULAR_ENV is not set
	origEnv := os.Getenv("TENTACULAR_ENV")
	_ = os.Unsetenv("TENTACULAR_ENV")
	defer func() { _ = os.Setenv("TENTACULAR_ENV", origEnv) }()

	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`default_env: staging
environments:
  staging:
    namespace: staging-ns
    mcp_endpoint: http://staging-mcp:8080
`), 0o644)

	env, err := ResolveEnvironment("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Namespace != "staging-ns" {
		t.Errorf("expected staging-ns from default_env, got %q", env.Namespace)
	}
	if env.MCPEndpoint != "http://staging-mcp:8080" {
		t.Errorf("expected staging mcp_endpoint from default_env, got %q", env.MCPEndpoint)
	}
}

// TestResolveEnvironmentDefaultEnvOverriddenByTENTACULAR_ENV verifies that
// TENTACULAR_ENV takes priority over default_env.
func TestResolveEnvironmentDefaultEnvOverriddenByTENTACULAR_ENV(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	origEnv := os.Getenv("TENTACULAR_ENV")
	_ = os.Setenv("TENTACULAR_ENV", "prod")
	defer func() { _ = os.Setenv("TENTACULAR_ENV", origEnv) }()

	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`default_env: dev
environments:
  dev:
    namespace: dev-ns
  prod:
    namespace: prod-ns
`), 0o644)

	// TENTACULAR_ENV=prod should override default_env=dev
	env, err := ResolveEnvironment("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Namespace != "prod-ns" {
		t.Errorf("expected prod-ns (TENTACULAR_ENV wins over default_env), got %q", env.Namespace)
	}
}

// TestEnvironmentTokenPathFieldIsIgnored documents that `token_path` at the
// environment level is NOT a valid field. The correct field name is
// `mcp_token_path`. Using `token_path` (which belongs to the global mcp block)
// silently produces an empty MCPTokenPath, leading to auth failures.
func TestEnvironmentTokenPathFieldIsIgnored(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	_ = os.Unsetenv("TENTACULAR_ENV")

	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)

	// WRONG: token_path is not a recognized field in the environment block.
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`default_env: eastus-dev
environments:
  eastus-dev:
    mcp_endpoint: https://mcp.eastus-dev1.example.com
    token_path: ~/dev-secrets/eastus-auth-token
    namespace: tent-dev
`), 0o644)

	env, err := ResolveEnvironment("eastus-dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// token_path is silently ignored; MCPTokenPath should be empty.
	if env.MCPTokenPath != "" {
		t.Errorf("expected MCPTokenPath to be empty when using wrong field name token_path, got %q", env.MCPTokenPath)
	}
}

// TestEnvironmentMCPTokenPathWithTildeExpansion verifies a complete config
// with mcp_token_path at the environment level resolves correctly, including
// tilde expansion to the user's home directory.
func TestEnvironmentMCPTokenPathWithTildeExpansion(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	_ = os.Unsetenv("TENTACULAR_ENV")

	// Write a token file in the fake home
	secretsDir := filepath.Join(tmpHome, "dev-secrets", "tentacular-mcp")
	_ = os.MkdirAll(secretsDir, 0o755)
	tokenFile := filepath.Join(secretsDir, "eastus-auth-token")
	_ = os.WriteFile(tokenFile, []byte("test-bearer-token\n"), 0o600)

	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)

	// CORRECT: mcp_token_path is the right field name for per-env token paths.
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`default_env: eastus-dev
environments:
  eastus-dev:
    mcp_endpoint: https://mcp.eastus-dev1.example.com
    mcp_token_path: ~/dev-secrets/tentacular-mcp/eastus-auth-token
    namespace: tent-dev
`), 0o644)

	env, err := ResolveEnvironment("eastus-dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify tilde expansion happened
	expectedPath := filepath.Join(tmpHome, "dev-secrets", "tentacular-mcp", "eastus-auth-token")
	if env.MCPTokenPath != expectedPath {
		t.Errorf("expected MCPTokenPath=%s after tilde expansion, got %s", expectedPath, env.MCPTokenPath)
	}

	// Verify the expanded path actually resolves to a readable token file
	token, err := readTokenFile(env.MCPTokenPath)
	if err != nil {
		t.Fatalf("failed to read token file at expanded path: %v", err)
	}
	if token != "test-bearer-token" {
		t.Errorf("expected token 'test-bearer-token', got %q", token)
	}
}

// TestGlobalMCPTokenPathTildeExpansion verifies that the global mcp.token_path
// also gets tilde expansion when used as a fallback.
func TestGlobalMCPTokenPathTildeExpansion(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	_ = os.Unsetenv("TENTACULAR_ENV")
	_ = os.Unsetenv("TNTC_MCP_ENDPOINT")
	_ = os.Unsetenv("TNTC_MCP_TOKEN")

	// Write a token file
	tokenFile := filepath.Join(tmpHome, "global-mcp-token")
	_ = os.WriteFile(tokenFile, []byte("global-token-value\n"), 0o600)

	userDir := filepath.Join(tmpHome, ".tentacular")
	_ = os.MkdirAll(userDir, 0o755)
	_ = os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(`mcp:
  endpoint: http://global-mcp:8080
  token_path: ~/global-mcp-token
environments:
  dev:
    namespace: dev-ns
`), 0o644)

	// Load config and verify the global token_path is stored as-is (no expansion at load time)
	cfg := LoadConfig()
	if cfg.MCP.TokenPath != "~/global-mcp-token" {
		t.Errorf("expected raw mcp.token_path='~/global-mcp-token', got %q", cfg.MCP.TokenPath)
	}

	// Verify expandHome works on the global path
	expanded := expandHome(cfg.MCP.TokenPath)
	expectedPath := filepath.Join(tmpHome, "global-mcp-token")
	if expanded != expectedPath {
		t.Errorf("expected expanded path=%s, got %s", expectedPath, expanded)
	}

	// Verify the token file is readable at the expanded path
	token, err := readTokenFile(expanded)
	if err != nil {
		t.Fatalf("failed to read global token file: %v", err)
	}
	if token != "global-token-value" {
		t.Errorf("expected token 'global-token-value', got %q", token)
	}
}
