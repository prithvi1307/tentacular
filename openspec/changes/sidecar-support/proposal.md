## Why

Tentacular workflows run TypeScript/Deno inside a distroless container under gVisor. There is no mechanism for executing native binaries (ffmpeg, ImageMagick, ML models, headless browsers). Research validated that sidecars are the best approach: native performance (5-10x faster than WASM), any Docker image works, on-demand multi-request processing, and the engine sandbox stays untouched. gVisor covers all containers in the pod. Full PSA restricted profile is compatible.

This change adds sidecar container support to the workflow spec, parser, Deno flag derivation, and Kubernetes builder so that any workflow can declare sidecar containers alongside the Deno engine.

## What Changes

### New Types

- `SidecarSpec` struct with fields: `Name`, `Image`, `Command`, `Args`, `Env`, `Port`, `Protocol`, `HealthPath`, `Resources`
- `ResourceSpec` and `ResourceValues` structs for sidecar resource requests/limits
- `Sidecars []SidecarSpec` field added to the `Workflow` struct
- TypeScript `SidecarSpec` interface in `engine/types.ts`

### Parser Validation

- `validateSidecars()` called from `Parse()`:
  - Name must match `identRe`
  - Image required, non-empty
  - Port required, range 1024-65535, must not be 8080 (reserved for engine)
  - No duplicate sidecar names
  - No duplicate sidecar ports
  - Protocol (if set) must be "http" or "grpc"

### Deno Flag Derivation

- `DeriveDenoFlags` signature updated to accept `sidecars []SidecarSpec` parameter
- Adds `localhost:PORT` to `allowedHosts` for each declared sidecar
- All existing call sites updated

### Kubernetes Builder

- `buildSidecarContainers()` helper generates container YAML per sidecar
- Shared `emptyDir` volume at `/shared` mounted in engine and all sidecar containers (when sidecars present)
- Per-sidecar `/tmp` emptyDir volume (required for tools like ffmpeg)
- Each sidecar gets identical `SecurityContext`: `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities: drop: ["ALL"]`
- HTTP readiness probe on `healthPath` (default `/health`) at sidecar port
- Pod-level security (runAsNonRoot, runAsUser, seccompProfile, runtimeClassName) applies to all containers

## Requirements

1. The `SidecarSpec` type must support name, image, command, args, env, port, protocol, healthPath, and resources fields
2. Sidecar names must match the existing `identRe` pattern used for node names
3. Sidecar ports must be in range 1024-65535 and must not be 8080 (engine port)
4. No duplicate sidecar names or ports within a workflow
5. Protocol field must be "http" (default) or "grpc" when specified
6. `DeriveDenoFlags` must add `localhost:PORT` for each sidecar to Deno `--allow-net` flags
7. Builder must generate multi-container pod specs with shared emptyDir at `/shared`
8. Each sidecar container must have the same SecurityContext as the engine container
9. Each sidecar must have a readiness probe on its health endpoint
10. Existing workflows without `sidecars:` must parse identically (backwards compatible)

## Acceptance Criteria

- [ ] `SidecarSpec`, `ResourceSpec`, and `ResourceValues` types exist in `pkg/spec/types.go`
- [ ] `Workflow` struct has `Sidecars []SidecarSpec` field
- [ ] Parser validates sidecar name matches `identRe`
- [ ] Parser rejects port outside 1024-65535
- [ ] Parser rejects port 8080
- [ ] Parser rejects duplicate sidecar names
- [ ] Parser rejects duplicate sidecar ports
- [ ] Parser rejects invalid protocol values
- [ ] Parser accepts workflows without `sidecars:` field unchanged
- [ ] `DeriveDenoFlags` includes `localhost:PORT` for each sidecar in `--allow-net`
- [ ] `DeriveDenoFlags` with no sidecars produces identical output to current behavior
- [ ] Builder generates multi-container pod with engine + sidecar containers
- [ ] Builder adds shared emptyDir volume at `/shared` when sidecars present
- [ ] Builder adds per-sidecar `/tmp` emptyDir
- [ ] Builder sets identical SecurityContext on sidecar containers
- [ ] Builder adds HTTP readiness probe for each sidecar
- [ ] Builder output unchanged when no sidecars declared
- [ ] TypeScript `SidecarSpec` interface exists in `engine/types.ts`
- [ ] ~14 unit tests covering all validation paths and builder output
- [ ] `go test ./pkg/spec/... ./pkg/builder/...` passes
- [ ] `golangci-lint run ./...` passes

## Scope

### In Scope

- SidecarSpec type definition (Go + TypeScript)
- Parser validation for all sidecar fields
- DeriveDenoFlags update for sidecar localhost ports
- K8s builder multi-container pod generation
- Shared volume and per-sidecar /tmp volume
- SecurityContext propagation to sidecar containers
- Readiness probes for sidecars
- Unit tests for all new code paths

### Out of Scope

- MCP server changes (no new tools needed for sidecars)
- Sidecar image registry or curation
- GPU resource support (future enhancement)
- Init container support (separate feature)
- NetworkPolicy changes for sidecar-to-external traffic (handled by existing contract dependencies)
- Runtime validation that sidecar containers actually start

## Dependencies

- None (this is the foundation that other repos depend on)

## Downstream Dependents

- `tentacular-skill/openspec/changes/sidecar-support/` -- skill docs reference the spec schema
- `tentacular-scaffolds/openspec/changes/sidecar-scaffolds/` -- scaffolds use the `sidecars:` spec field
- `tentacular-docs/openspec/changes/sidecar-support/` -- docs reference spec, skill, and scaffolds
