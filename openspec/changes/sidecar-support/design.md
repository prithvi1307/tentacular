# Design: Sidecar Container Support — tentacular/

## Overview

Add sidecar container support to the workflow spec, parser, Deno flag derivation, builder, and engine types. This enables workflows to declare auxiliary containers (ffmpeg, headless browsers, ML models, etc.) that run alongside the Deno engine in the same pod, communicating via localhost HTTP and shared volumes.

All changes are backwards compatible. Existing workflows without `sidecars:` parse identically.

---

## 1. Type Definitions (`pkg/spec/types.go`)

Add the following types after the existing `EgressOverride` struct (after line 114):

```go
// SidecarSpec declares an auxiliary container that runs alongside the engine.
type SidecarSpec struct {
	Name       string            `yaml:"name"`
	Image      string            `yaml:"image"`
	Command    []string          `yaml:"command,omitempty"`
	Args       []string          `yaml:"args,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
	Port       int               `yaml:"port"`
	Protocol   string            `yaml:"protocol,omitempty"`   // "http" (default) or "grpc"
	HealthPath string            `yaml:"healthPath,omitempty"` // readiness probe path, default "/health"
	Resources  *ResourceSpec     `yaml:"resources,omitempty"`
}

// ResourceSpec declares resource requests and limits for a container.
type ResourceSpec struct {
	Requests ResourceValues `yaml:"requests,omitempty"`
	Limits   ResourceValues `yaml:"limits,omitempty"`
}

// ResourceValues holds CPU and memory resource quantities.
type ResourceValues struct {
	CPU    string `yaml:"cpu,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}
```

Add `Sidecars` field to the `Workflow` struct (insert between `Deployment` and `Config` fields, around line 11):

```go
type Workflow struct {
	Metadata    *WorkflowMetadata   `yaml:"metadata,omitempty"`
	Contract    *Contract           `yaml:"contract,omitempty"`
	Nodes       map[string]NodeSpec `yaml:"nodes"`
	Name        string              `yaml:"name"`
	Version     string              `yaml:"version"`
	Description string              `yaml:"description"`
	Deployment  DeploymentConfig    `yaml:"deployment,omitempty"`
	Sidecars    []SidecarSpec       `yaml:"sidecars,omitempty"`
	Config      WorkflowConfig      `yaml:"config"`
	Triggers    []Trigger           `yaml:"triggers"`
	Edges       []Edge              `yaml:"edges"`
}
```

**Design rationale:** Sidecars are top-level in the workflow spec, not inside the contract. The contract declares *what external services the pod can reach* (driving NetworkPolicy and `--allow-net`). Sidecars declare *what containers run in the pod* — orthogonal concerns. A sidecar that needs external access gets it through a contract dependency.

---

## 2. Validation Rules (`pkg/spec/parse.go`)

Add a `validateSidecars` function and call it from `Parse()`. Insert the call after the contract validation block (after line 131), before the final error check:

```go
// In Parse(), after contract validation:
if len(wf.Sidecars) > 0 {
    if sidecarErrs := validateSidecars(wf.Sidecars); len(sidecarErrs) > 0 {
        errs = append(errs, sidecarErrs...)
    }
}
```

Add `validateSidecars` function (after the `ValidateContract` function):

```go
// validateSidecars validates the sidecars section of a workflow spec.
func validateSidecars(sidecars []SidecarSpec) []string {
	var errs []string
	names := make(map[string]bool)
	ports := make(map[int]bool)

	for i, sc := range sidecars {
		prefix := fmt.Sprintf("sidecars[%d]", i)

		// Name: required, must match identRe
		if sc.Name == "" {
			errs = append(errs, fmt.Sprintf("%s: name is required", prefix))
		} else if !identRe.MatchString(sc.Name) {
			errs = append(errs, fmt.Sprintf("%s: name must match [a-z][a-z0-9_-]*, got: %q", prefix, sc.Name))
		} else {
			if names[sc.Name] {
				errs = append(errs, fmt.Sprintf("%s: duplicate sidecar name %q", prefix, sc.Name))
			}
			names[sc.Name] = true
		}

		// Image: required, non-empty
		if sc.Image == "" {
			errs = append(errs, fmt.Sprintf("%s: image is required", prefix))
		}

		// Port: required, 1024-65535, not 8080 (engine port)
		if sc.Port == 0 {
			errs = append(errs, fmt.Sprintf("%s: port is required", prefix))
		} else if sc.Port < 1024 || sc.Port > 65535 {
			errs = append(errs, fmt.Sprintf("%s: port must be 1024-65535, got: %d", prefix, sc.Port))
		} else if sc.Port == 8080 {
			errs = append(errs, fmt.Sprintf("%s: port 8080 is reserved for the engine", prefix))
		} else {
			if ports[sc.Port] {
				errs = append(errs, fmt.Sprintf("%s: duplicate port %d", prefix, sc.Port))
			}
			ports[sc.Port] = true
		}

		// Protocol: if set, must be "http" or "grpc"
		if sc.Protocol != "" && sc.Protocol != "http" && sc.Protocol != "grpc" {
			errs = append(errs, fmt.Sprintf("%s: protocol must be \"http\" or \"grpc\", got: %q", prefix, sc.Protocol))
		}
	}

	return errs
}
```

**Validation rules summary:**

| Field | Rule | Error message pattern |
|-------|------|-----------------------|
| `name` | Required, matches `identRe` (`^[a-z][a-z0-9_-]*$`) | `sidecars[N]: name ...` |
| `name` | No duplicates within workflow | `sidecars[N]: duplicate sidecar name "X"` |
| `image` | Required, non-empty | `sidecars[N]: image is required` |
| `port` | Required, range 1024-65535 | `sidecars[N]: port must be 1024-65535` |
| `port` | Must not be 8080 (engine) | `sidecars[N]: port 8080 is reserved for the engine` |
| `port` | No duplicates within workflow | `sidecars[N]: duplicate port N` |
| `protocol` | If set, must be `"http"` or `"grpc"` | `sidecars[N]: protocol must be ...` |

**HealthPath default:** Not enforced in validation. The builder applies `"/health"` as default when generating the readiness probe (see section 4).

---

## 3. DeriveDenoFlags Changes (`pkg/spec/derive.go`)

### New function signature

Change the `DeriveDenoFlags` signature from:

```go
func DeriveDenoFlags(c *Contract, proxyHost string) []string
```

to:

```go
func DeriveDenoFlags(c *Contract, sidecars []SidecarSpec, proxyHost string) []string
```

### Sidecar localhost entries

After the existing `allowedHosts` population loop (around line 317, after the `seen` map loop), add localhost entries for each sidecar:

```go
// Add localhost:PORT for each declared sidecar
for _, sc := range sidecars {
    hostPort := "localhost:" + strconv.Itoa(sc.Port)
    if !seen[hostPort] {
        allowedHosts = append(allowedHosts, hostPort)
        seen[hostPort] = true
    }
}
```

This goes inside the `else` block (scoped mode), after the dependency host loop and before the module proxy host addition. In the `hasDynamic` case (broad `--allow-net`), sidecar ports are already included because `--allow-net` with no value allows all network access.

### Shared volume access flags

When sidecars are present, the engine needs read/write access to `/shared`. Update the flags section (around lines 341-343):

```go
// Determine read/write paths based on sidecar presence
allowReadFlag := "--allow-read=/app"
allowWriteFlag := "--allow-write=/tmp"
if len(sidecars) > 0 {
    allowReadFlag = "--allow-read=/app,/shared"
    allowWriteFlag = "--allow-write=/tmp,/shared"
}

flags := []string{
    "deno",
    "run",
    "--no-lock",
    "--unstable-net",
    allowNetFlag,
    allowReadFlag,
    allowWriteFlag,
    "--allow-env=DENO_DIR,HOME,SPIFFE_ENDPOINT_SOCKET,SPIFFE_ID,SPIFFE_ID_PATH,SVID_CERT_PATH,TELEMETRY_SINK",
}
```

### Nil sidecars handling

When `sidecars` is nil (no sidecars declared), the function behaves identically to the current implementation — no localhost entries added, no `/shared` paths.

### Early return condition

The existing early return `if c == nil || len(c.Dependencies) == 0` needs adjustment. When sidecars are present but no contract dependencies exist, we still need Deno flags for localhost access. Update:

```go
if (c == nil || len(c.Dependencies) == 0) && len(sidecars) == 0 {
    return nil
}
```

When there are sidecars but no contract, generate minimal flags with just the sidecar localhost entries and the health endpoint:

```go
if c == nil || len(c.Dependencies) == 0 {
    // Sidecars present but no contract — generate minimal flags
    var allowedHosts []string
    for _, sc := range sidecars {
        allowedHosts = append(allowedHosts, "localhost:"+strconv.Itoa(sc.Port))
    }
    allowedHosts = append(allowedHosts, "0.0.0.0:8080")
    sort.Strings(allowedHosts)

    return []string{
        "deno",
        "run",
        "--no-lock",
        "--unstable-net",
        "--allow-net=" + strings.Join(allowedHosts, ","),
        "--allow-read=/app,/shared",
        "--allow-write=/tmp,/shared",
        "--allow-env=DENO_DIR,HOME,SPIFFE_ENDPOINT_SOCKET,SPIFFE_ID,SPIFFE_ID_PATH,SVID_CERT_PATH,TELEMETRY_SINK",
        "engine/main.ts",
        "--workflow",
        "/app/workflow/workflow.yaml",
        "--port",
        "8080",
    }
}
```

### Updated call site (`pkg/builder/k8s.go`, line 246)

Change:
```go
denoFlags := spec.DeriveDenoFlags(wf.Contract, proxyHost)
```
to:
```go
denoFlags := spec.DeriveDenoFlags(wf.Contract, wf.Sidecars, proxyHost)
```

---

## 4. Builder Changes (`pkg/builder/k8s.go`)

### New helper: `buildSidecarContainers`

Add this function after the existing `buildDeployAnnotations` function:

```go
// buildSidecarContainers generates YAML for sidecar container specs.
// Each sidecar gets the same SecurityContext hardening as the engine container.
// Returns the YAML string to inject into the containers section (8-space base indent).
func buildSidecarContainers(sidecars []spec.SidecarSpec) string {
	if len(sidecars) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, sc := range sidecars {
		// Container name and image
		sb.WriteString(fmt.Sprintf("        - name: %s\n", sc.Name))
		sb.WriteString(fmt.Sprintf("          image: %s\n", sc.Image))
		sb.WriteString("          imagePullPolicy: Always\n")

		// Command (optional)
		if len(sc.Command) > 0 {
			sb.WriteString("          command:\n")
			for _, c := range sc.Command {
				sb.WriteString(fmt.Sprintf("            - %s\n", c))
			}
		}

		// Args (optional)
		if len(sc.Args) > 0 {
			sb.WriteString("          args:\n")
			for _, a := range sc.Args {
				sb.WriteString(fmt.Sprintf("            - %s\n", a))
			}
		}

		// Env (optional)
		if len(sc.Env) > 0 {
			sb.WriteString("          env:\n")
			// Sort keys for deterministic output
			envKeys := make([]string, 0, len(sc.Env))
			for k := range sc.Env {
				envKeys = append(envKeys, k)
			}
			sort.Strings(envKeys)
			for _, k := range envKeys {
				sb.WriteString(fmt.Sprintf("            - name: %s\n", k))
				sb.WriteString(fmt.Sprintf("              value: %s\n", sc.Env[k]))
			}
		}

		// Port
		sb.WriteString("          ports:\n")
		sb.WriteString(fmt.Sprintf("            - containerPort: %d\n", sc.Port))
		sb.WriteString("              protocol: TCP\n")

		// SecurityContext (same hardening as engine)
		sb.WriteString("          securityContext:\n")
		sb.WriteString("            readOnlyRootFilesystem: true\n")
		sb.WriteString("            allowPrivilegeEscalation: false\n")
		sb.WriteString("            capabilities:\n")
		sb.WriteString("              drop:\n")
		sb.WriteString("                - ALL\n")

		// Readiness probe
		healthPath := sc.HealthPath
		if healthPath == "" {
			healthPath = "/health"
		}
		sb.WriteString("          readinessProbe:\n")
		sb.WriteString("            httpGet:\n")
		sb.WriteString(fmt.Sprintf("              path: %s\n", healthPath))
		sb.WriteString(fmt.Sprintf("              port: %d\n", sc.Port))
		sb.WriteString("            initialDelaySeconds: 3\n")
		sb.WriteString("            periodSeconds: 5\n")

		// Volume mounts: /shared + /tmp
		sb.WriteString("          volumeMounts:\n")
		sb.WriteString("            - name: shared\n")
		sb.WriteString("              mountPath: /shared\n")
		sb.WriteString(fmt.Sprintf("            - name: tmp-%s\n", sc.Name))
		sb.WriteString("              mountPath: /tmp\n")

		// Resources (optional)
		if sc.Resources != nil {
			sb.WriteString("          resources:\n")
			if sc.Resources.Requests.CPU != "" || sc.Resources.Requests.Memory != "" {
				sb.WriteString("            requests:\n")
				if sc.Resources.Requests.Memory != "" {
					sb.WriteString(fmt.Sprintf("              memory: \"%s\"\n", sc.Resources.Requests.Memory))
				}
				if sc.Resources.Requests.CPU != "" {
					sb.WriteString(fmt.Sprintf("              cpu: \"%s\"\n", sc.Resources.Requests.CPU))
				}
			}
			if sc.Resources.Limits.CPU != "" || sc.Resources.Limits.Memory != "" {
				sb.WriteString("            limits:\n")
				if sc.Resources.Limits.Memory != "" {
					sb.WriteString(fmt.Sprintf("              memory: \"%s\"\n", sc.Resources.Limits.Memory))
				}
				if sc.Resources.Limits.CPU != "" {
					sb.WriteString(fmt.Sprintf("              cpu: \"%s\"\n", sc.Resources.Limits.CPU))
				}
			}
		}
	}
	return sb.String()
}
```

### New helper: `buildSidecarVolumes`

```go
// buildSidecarVolumes generates volume YAML for sidecar support.
// Creates a shared emptyDir + per-sidecar /tmp emptyDir volumes.
// Returns the YAML string to append to the volumes section (8-space base indent).
func buildSidecarVolumes(sidecars []spec.SidecarSpec) string {
	if len(sidecars) == 0 {
		return ""
	}

	var sb strings.Builder
	// Shared emptyDir for engine <-> sidecar file handoff
	sb.WriteString("        - name: shared\n")
	sb.WriteString("          emptyDir:\n")
	sb.WriteString("            sizeLimit: 1Gi\n")
	// Per-sidecar /tmp volumes (needed for tools like ffmpeg)
	for _, sc := range sidecars {
		sb.WriteString(fmt.Sprintf("        - name: tmp-%s\n", sc.Name))
		sb.WriteString("          emptyDir:\n")
		sb.WriteString("            sizeLimit: 256Mi\n")
	}
	return sb.String()
}
```

### Integration into `GenerateK8sManifests`

The existing function uses a large `fmt.Sprintf` template. The integration points are:

**a) Sidecar containers block** — inject after the engine container's closing resource block. Build the sidecar YAML string before the Sprintf:

```go
// Build sidecar containers block (empty string if no sidecars)
sidecarContainersBlock := buildSidecarContainers(wf.Sidecars)
```

Then add `%s` in the template after the engine container's resources section (after the `cpu: "500m"` line) and pass `sidecarContainersBlock` as the corresponding argument.

**b) Shared volume mount on engine** — when sidecars are present, add `/shared` mount to the engine container:

```go
// Engine shared volume mount (only when sidecars declared)
engineSharedMount := ""
if len(wf.Sidecars) > 0 {
    engineSharedMount = "            - name: shared\n              mountPath: /shared\n"
}
```

Insert this in the template after the engine's existing volume mounts (after the `/tmp` mount) as a `%s` placeholder.

**c) Sidecar volumes** — append after the existing volumes section:

```go
sidecarVolumesBlock := buildSidecarVolumes(wf.Sidecars)
```

Add `%s` at the end of the volumes section and pass `sidecarVolumesBlock`.

### Template format string changes

The modified `fmt.Sprintf` call will have these additional `%s` placeholders (shown in context):

```
          ...
          volumeMounts:
            - name: code
              mountPath: /app/workflow
              readOnly: true
            - name: secrets
              mountPath: /app/secrets
              readOnly: true
            - name: tmp
              mountPath: /tmp
%s%s          resources:          <-- engineSharedMount + importMapVolumeMount
            ...
            cpu: "500m"
%s      volumes:                  <-- sidecarContainersBlock
        ...
        - name: tmp
          emptyDir:
            sizeLimit: 512Mi
%s%s                              <-- sidecarVolumesBlock + importMapVolume
```

---

## 5. Engine Types (`engine/types.ts`)

Add `SidecarSpec` interface and `sidecars` field to `WorkflowSpec`. Insert the interface after `ContractSpec` (after line 19):

```typescript
/** Sidecar container that runs alongside the engine in the same pod */
export interface SidecarSpec {
  name: string;
  image: string;
  command?: string[];
  args?: string[];
  env?: Record<string, string>;
  port: number;
  protocol?: "http" | "grpc";
  healthPath?: string;
  resources?: {
    requests?: { cpu?: string; memory?: string };
    limits?: { cpu?: string; memory?: string };
  };
}
```

Add `sidecars` field to `WorkflowSpec` (after `contract?` field, line 12):

```typescript
export interface WorkflowSpec {
  name: string;
  version: string;
  description?: string;
  triggers: Trigger[];
  nodes: Record<string, NodeSpec>;
  edges: Edge[];
  config?: WorkflowConfig;
  contract?: ContractSpec;
  sidecars?: SidecarSpec[];
}
```

**No engine runtime changes.** Nodes call sidecars via `globalThis.fetch("http://localhost:PORT/path")`. The `--allow-net` flags include `localhost:PORT` for each sidecar (handled by `DeriveDenoFlags`). No new `ctx.sidecar()` method is needed.

---

## 6. Test Plan

### `pkg/spec/parse_test.go` — Sidecar Validation Tests

| Test Function | Verifies |
|---------------|----------|
| `TestValidSidecar` | A workflow with valid sidecar parses without errors |
| `TestSidecarNameRequired` | Empty name produces error |
| `TestSidecarNameInvalid` | Name not matching `identRe` produces error |
| `TestSidecarImageRequired` | Empty image produces error |
| `TestSidecarPortRequired` | Port 0 produces error |
| `TestSidecarPortRange` | Port outside 1024-65535 produces error |
| `TestSidecarPortReserved` | Port 8080 produces error |
| `TestSidecarDuplicateName` | Two sidecars with same name produces error |
| `TestSidecarDuplicatePort` | Two sidecars with same port produces error |
| `TestSidecarProtocolInvalid` | Protocol other than "http"/"grpc" produces error |
| `TestSidecarProtocolOptional` | Empty protocol is valid (defaults to http) |

**Test pattern** — follows existing `Parse()` test style:

```go
func TestValidSidecar(t *testing.T) {
    yaml := `
name: sidecar-test
version: "1.0"
triggers:
  - type: manual
nodes:
  fetch:
    path: ./nodes/fetch.ts
sidecars:
  - name: ffmpeg
    image: ghcr.io/randybias/tentacular-ffmpeg-sidecar:v1.0.0
    port: 9000
`
    wf, errs := spec.Parse([]byte(yaml))
    if len(errs) > 0 {
        t.Fatalf("unexpected errors: %v", errs)
    }
    if len(wf.Sidecars) != 1 {
        t.Fatalf("expected 1 sidecar, got %d", len(wf.Sidecars))
    }
    if wf.Sidecars[0].Name != "ffmpeg" {
        t.Errorf("expected sidecar name ffmpeg, got %s", wf.Sidecars[0].Name)
    }
}
```

### `pkg/spec/derive_test.go` — Deno Flag Tests

| Test Function | Verifies |
|---------------|----------|
| `TestDeriveDenoFlagsSidecarLocalhost` | `localhost:9000` appears in `--allow-net` when sidecar declared |
| `TestDeriveDenoFlagsSidecarSharedVolume` | `--allow-read` includes `/shared` and `--allow-write` includes `/shared` |
| `TestDeriveDenoFlagsMultipleSidecars` | Multiple `localhost:PORT` entries in `--allow-net` |
| `TestDeriveDenoFlagsSidecarsNoContract` | Sidecars without contract still generates flags with localhost entries |
| `TestDeriveDenoFlagsDynamicTargetWithSidecars` | Dynamic target uses broad `--allow-net`, sidecars don't change it |

### `pkg/builder/k8s_test.go` — Builder Tests

Following existing `makeTestWorkflow` + `strings.Contains` pattern:

| Test Function | Verifies |
|---------------|----------|
| `TestK8sManifestSidecarContainer` | Sidecar container appears in deployment YAML with correct name/image/port |
| `TestK8sManifestSidecarSecurityContext` | Sidecar has `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`, `drop: ALL` |
| `TestK8sManifestSidecarReadinessProbe` | Sidecar has readiness probe on correct port and health path |
| `TestK8sManifestSidecarDefaultHealthPath` | When `healthPath` omitted, readiness probe uses `/health` |
| `TestK8sManifestSidecarCustomHealthPath` | Custom `healthPath` is used in readiness probe |
| `TestK8sManifestSharedVolume` | `shared` emptyDir volume exists when sidecars declared |
| `TestK8sManifestSharedVolumeOnEngine` | Engine container has `/shared` mount when sidecars declared |
| `TestK8sManifestNoSharedVolumeWithoutSidecars` | No `shared` volume when no sidecars (backwards compat) |
| `TestK8sManifestSidecarTmpVolume` | Per-sidecar `/tmp` emptyDir exists (named `tmp-<sidecar-name>`) |
| `TestK8sManifestSidecarResources` | Custom resources appear when specified |
| `TestK8sManifestSidecarEnv` | Env vars appear when specified |
| `TestK8sManifestMultipleSidecars` | Two sidecars both appear in the deployment |

**Test helper** — extend `makeTestWorkflow` with a sidecar variant:

```go
func makeTestWorkflowWithSidecar(name string) *spec.Workflow {
    wf := makeTestWorkflow(name)
    wf.Sidecars = []spec.SidecarSpec{
        {
            Name:       "ffmpeg",
            Image:      "ghcr.io/randybias/tentacular-ffmpeg-sidecar:v1.0.0",
            Port:       9000,
            HealthPath: "/health",
        },
    }
    return wf
}
```

---

## 7. Cross-Cutting Concerns

### Backwards Compatibility

- `Sidecars` field is `omitempty` — existing YAML without `sidecars:` unmarshals to `nil`
- `DeriveDenoFlags` with `nil` sidecars behaves identically to current
- `buildSidecarContainers` and `buildSidecarVolumes` return empty string for nil/empty sidecars
- No changes to the Service manifest
- No changes to ConfigMap generation

### Security

- Every sidecar gets identical SecurityContext to the engine (set in `buildSidecarContainers`)
- Pod-level `runAsNonRoot`, `runAsUser: 65534`, `seccompProfile: RuntimeDefault` apply to all containers
- Pod-level `runtimeClassName` (gVisor) covers all containers
- NetworkPolicy (derived from contract) covers the entire pod — sidecar external access requires a contract dependency
- Sidecar images are user-specified — image trust is the user's responsibility

### File Changes Summary

| File | Action | Lines Changed (est.) |
|------|--------|---------------------|
| `pkg/spec/types.go` | Edit — add 3 types + 1 field | +25 |
| `pkg/spec/parse.go` | Edit — add `validateSidecars()` + call site | +45 |
| `pkg/spec/derive.go` | Edit — new signature, sidecar ports, `/shared` paths | +30 |
| `pkg/builder/k8s.go` | Edit — 2 helpers + template integration | +110 |
| `pkg/builder/k8s_test.go` | Edit — ~12 test functions | +200 |
| `pkg/spec/parse_test.go` | Edit — ~11 test functions | +150 |
| `pkg/spec/derive_test.go` | Edit — ~5 test functions | +80 |
| `engine/types.ts` | Edit — 1 interface + 1 field | +15 |
