# ForgeDeck

> Self-hosted DevOps Control Plane for Docker-based applications

ForgeDeck คือแพลตฟอร์ม DevOps แบบ Self-hosted สำหรับจัดการ Application, Docker Server, Git Repository, CI/CD Pipeline และ Deployment จาก Dashboard เดียว

เป้าหมายคือทำให้การ Deploy Application ที่ใช้ Docker เป็นเรื่องง่าย โดยไม่ต้อง SSH เข้า Server เองทุกครั้ง

> Deploy your Docker applications without managing the infrastructure manually.

ไฟล์นี้เป็นภาพรวม รายละเอียดและสัญญา (contract) ที่ใช้ implement จริงอยู่ในเอกสารแยกด้านล่าง ถ้าเนื้อหาขัดกัน ให้ยึดเอกสารแยกเป็นหลัก

---

## เอกสาร

| ไฟล์ | เนื้อหา |
|---|---|
| [01 — Product Spec](01-product-spec.md) | ปัญหา, ผู้ใช้, MVP scope, สิ่งที่ไม่ทำ |
| [02 — Architecture](02-architecture.md) | Components, trust boundaries, job lifecycle |
| [03 — Database](03-database.md) | Schema, constraints, queue transaction |
| [04 — API Spec](04-api-spec.md) | REST API, SSE, Agent endpoints |
| [05 — Agent Protocol](05-agent-protocol.md) | Enrollment, heartbeat, snapshot, long-poll, job payloads |
| [06 — CI/CD Pipeline](06-ci-cd-pipeline.md) | Webhook → build → push → deploy → health check |
| [07 — Security](07-security.md) | Threat model และ production checklist |
| [08 — Frontend UX](08-frontend-ux.md) | Navigation, หน้าหลัก, design system, performance |
| [09 — Development Guide](09-development-guide.md) | Local setup, testing, PR checklist |
| [10 — Roadmap](10-roadmap.md) | Phases และ acceptance criteria |
| [11 — Implementation Plan](11-implementation-plan.md) | ข้อย่อยของแต่ละ phase และ commit |
| [12 — Measurements](12-measurements.md) | ผลวัด latency และทรัพยากร |
| [13 — Git Providers](13-git-providers.md) | ตั้งค่า GitHub App และ GitLab |
| [14 — Runner](14-runner.md) | Protocol, ขั้นตอน build และ isolation ของ Runner |
| [15 — Deployments](15-deployments.md) | Environment, deployment, release, rollback, service configuration |
| [16 — Backup and Restore](16-backup-restore.md) | สำรอง/กู้ database, key ที่ต้องเก็บแยก, ขั้นตอนทดสอบ |
| [AGENTS.md](../AGENTS.md) | กฎสำหรับ AI coding agents |

---

## ForgeDeck คืออะไร และไม่ใช่อะไร

**คือ** lightweight self-hosted control plane ที่ทำให้ flow นี้จบในที่เดียว:

```text
Code → Build → Test → Image → Registry → Deploy → Health Check → Release
```

**ไม่ใช่** Kubernetes platform, Git hosting, cloud provider หรือ general-purpose CI engine

ข้อได้เปรียบไม่ได้มาจากจำนวน feature แต่มาจาก:

```text
Simplicity + Speed + Self-hosted + Docker-native + Developer UX
```

---

## หลักการ

1. **Simple** — ผู้ใช้ไม่ต้องเข้าใจ infrastructure ทั้งหมดก็ deploy ได้
2. **Lightweight** — MVP ไม่ใช้ Kubernetes, Kafka, Redis, Temporal, Elasticsearch, Prometheus, Grafana
3. **Fast** — server-side pagination, SSE เฉพาะหน้าที่ต้องใช้, ไม่ polling ทั้งระบบ
4. **Secure by default** — ไม่เปิด Docker Engine เป็น public API, Agent เป็นตัวกลางเสมอ
5. **Developer friendly** — ทุกหน้าตอบได้ทันทีว่า "ตอนนี้ระบบเป็นอะไร"

---

## สถาปัตยกรรม

```text
              ┌────────────────────┐
              │      Browser       │
              │ React SPA + HeroUI │
              └─────────┬──────────┘
                        │
                        ▼
              ┌────────────────────┐        GitHub / GitLab
              │  Go Control Plane  │◄────── webhooks
              │ (modular monolith) │
              └──┬──────────────┬──┘
                 │              │
                 ▼              │ jobs (pull)
          ┌────────────┐        │
          │ PostgreSQL │   ┌────┴─────────────┐
          │ data+queue │   │                  │
          └────────────┘   ▼                  ▼
                       ┌────────┐       ┌──────────┐
                       │ Runner │       │  Agent   │  outbound HTTPS
                       └───┬────┘       └────┬─────┘  (long-poll)
                           │ push            │ pull
                           ▼                 ▼
                       ┌──────────┐   ┌───────────────┐
                       │ Registry │──►│ Docker Engine │
                       └──────────┘   │  App A, B, C  │
                                      └───────────────┘
```

| Component | หน้าที่ | รายละเอียด |
|---|---|---|
| Control Plane | Auth, projects, servers, jobs, deployments, secrets, audit | [02](02-architecture.md) |
| PostgreSQL | Source of truth และ job queue (`SKIP LOCKED`) | [03](03-database.md) |
| Runner | Checkout → test → build → push แยกจาก production | [06](06-ci-cd-pipeline.md) |
| Agent | ควบคุม Docker บน server, ส่ง heartbeat และ container snapshot | [05](05-agent-protocol.md) |
| Web UI | Vite + React SPA, HeroUI, TanStack Query — build เป็น static แล้ว embed ใน Go binary | [08](08-frontend-ux.md) |

หลักสำคัญ:

- **Server เป็นของ Project** — ตรวจสิทธิ์ทุกอย่างผ่าน `servers.project_id`
- **Agent ใช้ pull model** — server ไม่ต้องเปิด inbound port, อยู่หลัง NAT ได้
- **Runner แยกจาก Production** — source code เป็น untrusted input, ห้าม mount Docker socket
- **Deploy ด้วย image digest** — rollback ใช้ digest เดิม ไม่ build ใหม่
- **Control Plane เป็น binary เดียว** — Web UI ถูก embed ไว้ใน Go binary, production ไม่ต้องมี JavaScript runtime (ใช้ Bun แค่ตอน build)

---

## Golden Path

ถ้าผู้ใช้ใช้ค่า default ต้อง deploy ได้โดยไม่ต้อง configure อะไรมาก:

```text
Login → Create Project → Connect GitHub → Select Repository
  → Create Application → Add Server (install Agent)
  → Push Code → Watch Build → Watch Deployment → Healthy
```

---

## MVP Definition of Done

MVP ถือว่าสำเร็จเมื่อผู้ใช้ทำได้ครบ:

1. Login
2. สร้าง Project
3. Connect GitHub/GitLab
4. เลือก Repository
5. สร้าง Application
6. เพิ่ม Docker Server
7. Install Agent
8. Agent ขึ้น Online
9. Push Code
10. Webhook เข้า ForgeDeck
11. Runner รับ Job
12. Checkout Code
13. Run Test
14. Build Docker Image
15. Push Registry
16. Agent รับ Deployment
17. Pull Image
18. Start Container
19. Health Check
20. แสดง Deployment เป็น Healthy
21. ดู Logs
22. ดู Release History
23. Rollback Release

Milestone แรกคือ **"Manage a remote Docker server from the ForgeDeck Dashboard"** — ดูรายละเอียดแต่ละ phase ใน [10 — Roadmap](10-roadmap.md)

---

## Resource Goals

ForgeDeck (Control Plane + PostgreSQL + Runner) ควรรันได้บนเครื่อง **2 CPU / 4 GB RAM** สำหรับ development และ small production workload

Workload จริงของ application ขึ้นกับ server ที่ติดตั้ง Agent

---

## Repository Layout

Monorepo:

```text
forgedeck/
├── apps/web/          Vite + React dashboard (embed ใน cmd/api)
├── cmd/
│   ├── api/           Control Plane
│   ├── runner/        CI Runner
│   └── agent/         Docker Agent
├── internal/          Go modules (auth, projects, servers, agents, jobs, ...)
├── migrations/        SQL migrations
├── packages/api-client/
├── deploy/            Compose และ Runner setup
└── docs/
```

รายละเอียดและคำสั่งพัฒนาอยู่ใน [09 — Development Guide](09-development-guide.md)

---

## Philosophy

> Don't build infrastructure for the sake of infrastructure.

- ถ้า PostgreSQL ทำงานได้ ก็ไม่ต้องเพิ่ม Redis
- ถ้า Docker Compose ทำงานได้ ก็ไม่ต้องเพิ่ม Kubernetes
- ถ้า Agent + HTTPS ทำงานได้ ก็ไม่ต้องเพิ่ม Message Broker
- ถ้า SSE ทำงานได้ ก็ไม่ต้องเพิ่มระบบ Streaming ที่ซับซ้อน

> Simple until complexity is actually needed.

เป้าหมายคือระบบที่ **ใช้ง่ายที่สุด, เบาที่สุด, และ Deploy ได้จริง**
