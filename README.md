# Foliage FOSDEM 2026 Demo — Multi‑Model Digital Twin (m1–m4)
![License][License-Image]

[License-Url]: https://www.apache.org/licenses/LICENSE-2.0
[License-Image]: https://img.shields.io/badge/License-Apache2-blue.svg
This repository is a runnable demo project built on top of the **Foliage SDK**.  
The goal is to show how **multiple independently deployed “models”** (separate Foliage runtimes / domains) can ingest data from different sources, build their own object graphs, and then **cross‑link** into a single “bigger than any single team” knowledge view.

> diagrams go stale. This demo turns **real snapshots** (commands, Kubernetes state, architecture JSON) into a **living knowledge graph** and then intersects the graphs across domains.

---

## What you’ll see in the demo

- **m1 — Infrastructure twin**: servers → (KVM) hypervisors → VMs, built from command snapshots produced by lightweight host agents.
- **m2 — Kubernetes twin**: cluster → nodes → pods/deployments/replicasets, built from a real K8s API *or* a dump file.
- **m3 — Application architecture model**: an architect-owned JSON (Online Boutique microservices) turned into an object graph.
- **m4 — Datacenter “meta model”**: a thin “meta” adapter that discovers objects across **m1+m2+m3** and links them under a single root to enable global navigation and error propagation.

A key property of the system is **independent lifecycle**:
- You can start/stop/update m1 without touching m2/m3.
- Cross-domain linking is done via **shadow objects** and **weak clustering** (see below).

---

## Core concepts (Foliage terms used in this demo)

- **Domain / Model**: an isolated namespace of objects (here: `m1`, `m2`, `m3`, `m4`).
- **Connector**: ingests *raw* data into the CMDB (e.g., command stdout snapshots).
- **Adapter**: builds/updates a *derived* object model (digital twin) from connector sources.
- **Signal**: the event mechanism over NATS that drives incremental updates.
- **Weak cluster domains**: a configured set of “neighbor” domains that can be queried/linked via shadow objects.
- **Shadow objects**: local placeholders that point to real objects living in a different domain.

---

## Repository layout

```
common/                 Shared helpers (post-process notifier, error propagation, model names)
m1/                     Infrastructure model (KVM) + lab host emulators + agents
m2/                     Kubernetes model (connector+adapter in one service)
m3/                     Architecture model (JSON file connector + adapter)
m4/                     Datacenter meta-model (aggregates via shadow objects)
.github/workflows/      CI for building/publishing images (optional for the demo)
```

Each model directory contains:
- `cmd/` — the “command” runtime for that domain (health, utilities, and extra functions)
- `connectors/` and/or `cn_ad/` — ingestion services
- `adapters/` — derived-model services
- `docker-compose.yaml` — a runnable stack for that model

---

## Model-by-model overview

### m1 — Infrastructure (KVM) model

**Purpose:** Build a minimal infra digital twin and keep it fresh without humans manually drawing diagrams.

**Raw sources (connector objects):**
- `lshw -json` (server hardware snapshot)
- `lsmod` (kernel module list; used to detect KVM presence)
- `vagrant global-status --machine-readable` (VM inventory snapshot)
- `lshw -json` per VM (VM hardware snapshot; in the demo it is emulated)

**Connectors (3):**
- `cn_server`
    - function: `function.connector.server.lshw.push_update`
- `cn_hypervisor`
    - function: `function.connector.hypervisor.lsmod.push_update`
- `cn_virtual_machine`
    - functions:
        - `function.connector.virtual_machine.vagrant_global_status.push_update`
        - `function.connector.virtual_machine.lshw.push_update`

**Adapter (1):**
- `ad_infra`
    - function: `function.adapter.infra.push_update`
    - post-process: `function.adapter.infra.post_process`

**Digital twin objects produced by m1 adapter (high level):**
- `foliage-adapter-infra` (root)
- `foliage-adapter-server`
- `foliage-adapter-hypervisor` (only created if `lsmod` shows KVM modules)
- `foliage-adapter-virtual_machine`
- plus component objects parsed out of `lshw`: CPU, RAM sticks, disks, BIOS, serial number, network adapters, …

**Lab host emulation**
`m1/lab_hosts/*` contains two Ubuntu containers that emulate “bare metal” servers:
- run an **SSH server** (user `demo`/`demo`)
- run the **inventory agent** (`m1/lab_hosts/common/agent.sh`)
- host command dump data under `/opt/data/**`

---

### m2 — Kubernetes model

**Purpose:** Build a K8s object graph and **link it** to infra (m1) and architecture (m3).

**Input modes:**
- **Live mode**: talks to a cluster using a mounted kubeconfig.
- **Dump mode** (default in this repo): uses a YAML dump and creates a fake K8s client.

**Service:**
- `cn_ad_k8s` (connector+adapter in one runtime)
    - `function.cn_ad.k8s.push_update`
    - `function.cn_ad.k8s.post_process`

**Objects produced:**
- `foliage-k8s-infrastructure` (root)
- cluster, nodes, pods, deployments, replicasets

**Cross-model linking done in post-process:**
- **Node → VM (m1)**: creates shadow VM objects from m1 and links nodes to the right VM.
- **Pod/Deployment → ArchBlock (m3)**: creates shadow ArchBlock objects from m3 and links K8s workloads to the matching architecture blocks.

---

### m3 — Architecture model (JSON)

**Purpose:** Turn a human-owned architecture description into an object graph that can be connected to runtime reality.

**Connector:**
- `cn_json_file`
    - function: `function.connector.jsonfile.push_update`

**Adapter:**
- `ad_arch_model`
    - function: `function.adapter.arch_model.push_update`
    - post-process: `function.adapter.arch_model.post_process`

**How it is fed:**
- `m3/cmd` periodically reads `m3/cmd/data/online_boutique_architecture.json`
  and signals `cn_json_file` to ingest/update it.

---

### m4 — Datacenter meta-model

**Purpose:** Demonstrate a “meta” view: a small adapter that does not own raw data, but assembles a single navigable structure from multiple domains.

**Adapter:**
- `ad_datacenter`
    - post-process: `function.adapter.dc.post_process`

**What it does:**
- discovers objects from `m1`, `m2`, `m3` via weak clustering
- creates **shadow objects** and links them under a single root:
    - `datacenter`
        - `infra` (m1 shadow)
        - `k8s_infrastructure` (m2 shadow)
        - `arch_model` (m3 shadow)
- (optionally) enables graph-based error propagation across the whole stack

---

## Data flow (end-to-end)

### m1 (agents → connectors → adapter)
```
lab_host (agent.sh)
  ├─ runs commands (lshw / lsmod / vagrant global-status / VM lshw)
  └─ publishes raw JSON to NATS subjects:
       signal.<domain>.<function>.<connector_id>
            ↓
m1 connectors store raw sources as CMDB objects
            ↓
connectors signal adapter.infra.push_update with:
  {host_id, command, source_uuid, source_type}
            ↓
m1 adapter updates only the affected subgraph (incremental reconcile)
```

### m2 and m3
- m2 periodically snapshots K8s state (live or dump) and updates its object graph.
- m3 periodically reads architecture JSON and updates its graph.
- m2 post-process creates shadow links to m1 (VMs) and m3 (arch blocks).

### m4
- periodically runs post-process and re-links everything under a single root using shadow objects.

---

## Quickstart (local)

### Prerequisites
- Docker + Docker Compose
- (Optional) `nats` CLI if you want to publish signals manually

> **Important:** each model ships its own `docker-compose.yaml` with a NATS container and port mappings.  
> Running multiple stacks simultaneously **without modifications** will cause port conflicts.
> For a full multi-model run, point all models to a single shared NATS (see the next section).

### Run m1 (infra) locally
```bash
cd m1
docker compose up --build
```

To enable the lab hosts (if they are commented out in your local version), ensure `lab_host01` and `lab_host02` services are enabled in `m1/docker-compose.yaml`.

### Run m2 (k8s) locally
```bash
cd m2
docker compose up --build
```
By default it runs in **dump mode** and expects dumps under `m2/dumps/`.

### Run m3 (architecture) locally
```bash
cd m3
docker compose up --build
```
It will ingest `m3/cmd/data/online_boutique_architecture.json` automatically.

### Run m4 (datacenter meta view) locally
```bash
cd m4
docker compose up --build
```

---

## Running all models together (shared NATS)

To actually see cross-domain shadow linking (m2↔m1↔m3 and m4 aggregating all),
all models must connect to the **same** NATS/JetStream.

A practical approach for local experiments:

1) Start one NATS (pick any model’s `nats` service or run one manually).
2) For the other models:
    - disable their `nats` service in docker-compose (or remove port mappings)
    - set `NATS_URL` in their `*.env` files to point to the shared NATS
3) Ensure containers can reach that NATS address (either same Docker network or `host.docker.internal` on Docker Desktop).

This repo keeps per-model compose files intentionally isolated because the intended deployment is “each model is a separately shipped unit”.

---

## Notes & limitations

- This is a **demo repo**. Parsers are intentionally small and optimistic.
- IDs are chosen to be stable/human-friendly for presentations (e.g., `infra`, `k8s_infrastructure`, etc.).

---

## Contributing / extending the demo

Common extension points:
- add a new command snapshot in m1 (new connector function + new reconcile handler)
- enrich K8s linking rules in m2 post-process
- extend the architecture JSON schema and mapping in m3
- add more “meta” views or policies in m4 (blast radius analysis, ownership routing, incident playbooks, …)

---

## References

- Foliage SDK: https://github.com/foliagecp/sdk


## Ports (defaults)

The per-model compose files expose a few convenience ports:

- **NATS monitoring**: `28222 -> 8222` (conflicts across models if run simultaneously)
- **NATS WebSocket / TLS**: `10443 -> 443` (used by browser-based tools / UIs; conflicts across models)
- **cmd health endpoint**: some models map `9000 -> 9000`

If you run more than one model locally, you will need to adjust these mappings or use a shared NATS.

---

## Acknowledgements
This demo is built around NATS + the Foliage SDK and is intended for conference / community demonstrations.
