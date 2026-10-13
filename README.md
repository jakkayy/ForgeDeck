# ForgeDeck

> Self-hosted DevOps Control Plane สำหรับ Docker: Code → Build → Test → Image → Registry → Deploy

ForgeDeck จัดการ Application, Docker server, Git repository และ CI/CD จาก Dashboard เดียว โดยไม่ต้อง SSH เข้า server ทุกครั้ง เป็น Go binary เดียว (UI ฝังอยู่ข้างใน) ใช้ PostgreSQL เป็นทั้งฐานข้อมูลและคิวงาน

ภาพรวมและเหตุผลของการออกแบบอยู่ใน [docs/README.md](docs/README.md)

## สถานะ

| Phase | เนื้อหา | สถานะ |
|---|---|---|
| 0–1 | โครงโปรเจกต์, Auth, Project, Server + Agent, จัดการ container | เสร็จ |
| 2 | Git integration (GitHub/GitLab), webhook, job queue | เสร็จ |
| 3 | Runner แยกจาก production: checkout → build → test → push | เสร็จ |
| 4 | Environment, deploy ผ่าน Agent + Compose, health check, release, rollback | เสร็จ |
| 5–6 | Secrets, hardening, production readiness | ยังไม่เริ่ม |

รายละเอียดของแต่ละข้อดูที่ [docs/11-implementation-plan.md](docs/11-implementation-plan.md)

## ส่วนประกอบ

| ส่วน | หน้าที่ | Binary |
|---|---|---|
| Control Plane | API, Web UI, job queue, auth, secrets | `forgedeck-api` |
| Agent | อยู่บน Docker server ควบคุม container (เชื่อมออกมาหา Control Plane เอง) | `forgedeck-agent` |
| Runner | อยู่บน build host แยก ทำ checkout → build → test → push | `forgedeck-runner` |
| PostgreSQL | ข้อมูลและคิวงาน | ผ่าน Docker Compose |

## สิ่งที่ต้องมี

- Go (เวอร์ชันตาม `go.mod`), Docker Engine + Compose plugin, git
- [Bun](https://bun.sh) (ใช้ตอน build UI เท่านั้น ไม่ต้องใช้ตอนรัน)
- ตัวเลือก: `golangci-lint` v2.5 (`make lint`), `sqlc` v1.30 (`make generate`)

## เริ่มใช้งาน (โหมดพัฒนา)

```sh
cp .env.example .env     # ตั้ง POSTGRES_PASSWORD, FORGEDECK_DATABASE_URL, FORGEDECK_SECRET_KEYS
make db-up               # PostgreSQL
make dev-api             # Control Plane ที่ :8080 (migrate database ให้อัตโนมัติ)
make create-user EMAIL=you@example.com
make dev-web             # UI ที่ http://localhost:5173 (hot reload)
```

สร้าง key เข้ารหัส secret ด้วย `openssl rand -base64 32` แล้วใส่เป็น `FORGEDECK_SECRET_KEYS=v1:<ค่าที่ได้>` **เก็บ key นี้ไว้นอก database และสำรองไว้** ถ้าหาย จะอ่าน Git token ที่เก็บไว้ไม่ได้อีก

## รันแบบ binary เดียว

```sh
make build               # web + api + agent + runner ลงใน bin/
bin/forgedeck-api serve  # เปิด http://localhost:8080
```

## ต่อ Agent (จัดการ Docker ของเครื่องอื่น)

ใน UI ไปที่ **Servers > Add server** จะได้ token และคำสั่ง จากนั้นบนเครื่องปลายทาง:

```sh
bin/forgedeck-agent enroll --url https://forgedeck.example.com
bin/forgedeck-agent run
```

ถ้า Control Plane ไม่ใช่ HTTPS (เช่นทดลองใน LAN) ให้เพิ่ม `--allow-insecure`

## ต่อ Runner (build image)

```sh
make create-runner NAME=build-01          # แสดง credential (fdr_...) ครั้งเดียว
FORGEDECK_RUNNER_CREDENTIAL=fdr_... make run-runner URL=http://localhost:8080
```

`make run-runner` ใช้ Docker daemon ของเครื่องคุณโดยตรง เหมาะกับทดลองเท่านั้น การติดตั้งจริงบน build host ให้ดู [deploy/runner/README.md](deploy/runner/README.md) และ [docs/14-runner.md](docs/14-runner.md) เพราะ Runner รันโค้ดที่ไม่น่าไว้ใจ จึงควรอยู่บนเครื่องแยกที่ไม่มีอย่างอื่นสำคัญ

## ลอง build ครั้งแรก

1. สร้าง Project แล้วไปแท็บ **Git** เพื่อ Connect GitLab (token ต้องมี scope `api`) หรือ GitHub ([docs/13-git-providers.md](docs/13-git-providers.md))
2. แท็บ **Registries** เพิ่ม registry เช่น `ghcr.io` หรือ registry ในเครื่อง:
   `docker run -d --name registry -p 127.0.0.1:5000:5000 registry:3` แล้วใช้ host `localhost:5000`
3. แท็บ **Applications** สร้าง Application: เลือก repo, registry และชื่อ image
4. เปิดหน้า Application แล้วกด **Build** หรือ push โค้ดเข้า branch นั้น

ผล build คือ image reference แบบ `host/image@sha256:...` ที่ใช้ deploy ต่อ

## Deploy ครั้งแรก

1. เพิ่ม server ที่ **Servers** แล้วติดตั้ง Agent บนเครื่องนั้น (ต้องมี Docker และ Docker Compose plugin)
2. ในหน้า Application กด **New environment**: เลือก server, ใส่ container port / host port และ health check (ใต้ *Advanced*) เปิด *Deploy every successful build* ถ้าต้องการให้ build ที่สำเร็จขึ้น environment นี้เอง
3. กด **Deploy** จะเอา build ที่สำเร็จล่าสุดขึ้น ดูความคืบหน้า `Queued → Pulling → Starting → Health check → Healthy` ที่หน้า environment
4. ถ้าเวอร์ชันใหม่ไม่ healthy ระบบกู้เวอร์ชันก่อนหน้าให้เอง และกด **Roll back** ที่ Release history เพื่อกลับไป release เก่าได้ (**ข้อมูลใน volume และ database ไม่ถูกย้อน** ดู [docs/15-deployments.md](docs/15-deployments.md))

## ตัวแปรสำคัญ

ตัวอย่างทั้งหมดอยู่ใน [.env.example](.env.example)

| ตัวแปร | ความหมาย |
|---|---|
| `FORGEDECK_DATABASE_URL` | PostgreSQL connection string |
| `FORGEDECK_SECRET_KEYS` | key เข้ารหัส secret รูปแบบ `id:base64,id:base64` (ตัวแรกคือ key ที่ใช้เข้ารหัสใหม่) |
| `FORGEDECK_PUBLIC_URL` | URL ที่ GitHub/GitLab เรียก webhook กลับมา |
| `FORGEDECK_ALLOW_PRIVATE_GIT_HOSTS` | อนุญาต Git server ใน network ภายใน (ปิดไว้เป็นค่าเริ่มต้นเพื่อกัน SSRF) |

## คำสั่งที่ใช้บ่อย

| คำสั่ง | ทำอะไร |
|---|---|
| `make test` | รัน test ทั้งหมด (ต้องตั้ง `FORGEDECK_TEST_DATABASE_URL`; เพิ่ม `FORGEDECK_TEST_DOCKER=1` เพื่อรันเทสต์ที่ใช้ Docker) |
| `make lint` / `make fmt` | ตรวจ/จัดรูปแบบโค้ด Go |
| `make generate` | สร้างโค้ด sqlc จาก `internal/store/queries` |
| `make migrate-status` | ดูสถานะ migration |
| `make db-down` | หยุด PostgreSQL |

## เอกสาร

| ไฟล์ | เนื้อหา |
|---|---|
| [01 Product Spec](docs/01-product-spec.md) · [02 Architecture](docs/02-architecture.md) | ขอบเขตและสถาปัตยกรรม |
| [03 Database](docs/03-database.md) · [04 API](docs/04-api-spec.md) | schema และ REST API |
| [05 Agent Protocol](docs/05-agent-protocol.md) · [14 Runner](docs/14-runner.md) | protocol ของ Agent และ Runner |
| [06 CI/CD](docs/06-ci-cd-pipeline.md) · [07 Security](docs/07-security.md) | pipeline และ threat model |
| [08 Frontend](docs/08-frontend-ux.md) · [09 Development](docs/09-development-guide.md) | UI และคู่มือพัฒนา |
| [10 Roadmap](docs/10-roadmap.md) · [11 Plan](docs/11-implementation-plan.md) | แผนงาน |
| [12 Measurements](docs/12-measurements.md) · [13 Git Providers](docs/13-git-providers.md) | ผลวัดและการตั้งค่า provider |
| [15 Deployments](docs/15-deployments.md) | environment, deployment, release, rollback |
| [16 Backup and Restore](docs/16-backup-restore.md) | สำรองและกู้ข้อมูล, `FORGEDECK_SECRET_KEYS` ต้องเก็บแยก |
| [AGENTS.md](AGENTS.md) | กฎสำหรับ AI coding agents |
