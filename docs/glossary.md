# Glossary

Working glossary of Tentacular-specific terms. Will be unified into a central glossary when documentation moves to a dedicated repo.

| Term | Definition |
|------|-----------|
| **Tentacle** | A deployed workflow. The unit of work in Tentacular. |
| **Node** | A single TypeScript function within a tentacle. Nodes are connected by edges to form a DAG. |
| **Edge** | A directed connection between two nodes defining execution order and data flow. |
| **Trigger** | What initiates a workflow run: `manual`, `cron`, `webhook`, or `queue`. |
| **Contract** | The `contract:` section of workflow.yaml declaring external dependencies and network requirements. |
| **Dependency** | A service declared in the contract. `tentacular-*` prefixed deps are exoskeleton-managed. |
| **Workspace** | The exoskeleton-provisioned bundle of scoped resources for a tentacle (Postgres schema, NATS subjects, S3 prefix, SPIFFE identity). |
| **Exoskeleton** | Optional backing-service bundle managed by the MCP server. Provisions per-tentacle workspaces. |
| **Deploy gate** | Pre-deployment validation that checks contract drift, secret availability, and namespace readiness. |
| **Environment** | A named configuration context (e.g., `eastus-dev`, `prod`) with MCP endpoint, namespace, and optional OIDC settings. |
| **Profile** | Cluster-specific configuration resolved at deploy time (registry, runtime class, namespace). |
