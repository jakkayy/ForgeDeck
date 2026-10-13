# 11 — Implementation Plan

แผนลงมือสร้าง ForgeDeck ตั้งแต่ repository ว่างจนถึง MVP แบ่ง phase ตาม [10 — Roadmap](10-roadmap.md)

## กติกา

- ทำงานบน branch `develop` เท่านั้น เจ้าของ repo เป็นคน PR เข้า `main`
- **1 ข้อย่อย = 1 เรื่อง = 1 commit**
- Commit message บรรทัดเดียว ขึ้นต้นด้วย conventional prefix และไม่มี trailer ใดๆ
  - scope ที่ใช้: `api`, `agent`, `runner`, `web`, `db` เช่น `feat(api): add login endpoint`
- Unit test ของ feature อยู่ใน commit เดียวกับ feature นั้น ส่วน integration/E2E test แยกเป็นข้อของตัวเอง
- ทุก commit ต้องผ่าน `make lint` และ `make test` ณ จุดนั้น
- ถ้า implement แล้วพบว่า contract ใน `docs/` ต้องเปลี่ยน ให้แก้ docs ใน commit เดียวกัน
- ข้อที่มีเครื่องหมาย 🚧 คือ **decision gate** ต้องได้ข้อสรุปก่อนเริ่มข้อถัดไปใน phase นั้น

## ค่าเริ่มต้นที่เสนอ (ตรวจได้)

| เรื่อง | ที่เสนอ | เหตุผล |
|---|---|---|
| Go | 1.26 | ตรงกับเครื่อง dev ปัจจุบัน |
| Migration | `goose` (SQL files) | ไฟล์ SQL ล้วน ใช้กับ sqlc ได้ตรงๆ |
| Password hash | argon2id | ตาม [07](07-security.md#2-authentication-and-sessions) |
| Session | ตาราง `sessions` ใน PostgreSQL, cookie เก็บ token แบบสุ่ม (DB เก็บ hash) | revoke ได้ ไม่ต้องมี Redis |
| Docker SDK | `github.com/docker/docker/client` | official client |
| Frontend | Vite + React SPA embed ใน Go binary | ไม่มี JavaScript runtime ตอนรันจริง, origin เดียว, ติดตั้ง binary เดียว ([08](08-frontend-ux.md#2-stack)) |
| Package manager / build | Bun (ใช้ตอน build เท่านั้น) | ติดตั้ง dependency และรัน script เร็ว, ใช้รัน Vite ได้ |
| CI | GitHub Actions | remote อยู่บน GitHub |
| ผู้ใช้คนแรก | CLI `forgedeck-api create-user` ใน MVP (ไม่มี global admin, สิทธิ์อยู่ระดับ project), Setup Wizard ไว้ Phase 5 | ปิด open decision ใน [01](01-product-spec.md#10-open-product-decisions) แบบง่ายที่สุดก่อน |

---

## Phase 0 — Repository foundation

| # | งาน | Commit |
|---|---|---|
| 0.1 | ย้าย `AGENTS.md` ไปไว้ที่ root | `docs: move AGENTS.md to repository root` |
| 0.2 | เพิ่มตาราง `sessions` และ `enrollment_tokens` ใน database spec | `docs: add sessions and enrollment tokens to database spec` |
| 0.3 | สร้าง Go module | `chore: initialize Go module` |
| 0.4 | `.gitignore` และ `.editorconfig` | `chore: add gitignore and editorconfig` |
| 0.5 | `.env.example` (ชื่อตัวแปรและ placeholder เท่านั้น) | `chore: add env example` |
| 0.6 | `docker-compose.yml` สำหรับ PostgreSQL ตอน dev | `chore: add local PostgreSQL compose service` |
| 0.7 | golangci-lint config | `chore: add golangci-lint configuration` |
| 0.8 | Makefile: `fmt`, `lint`, `test` | `chore: add Makefile with fmt, lint and test targets` |
| 0.9 | GitHub Actions: lint + test สำหรับ Go | `ci: add Go lint and test workflow` |

---

## Phase 1 — Foundation and remote Docker visibility

### 1A. API skeleton

| # | งาน | Commit |
|---|---|---|
| 1.1 | โหลด config จาก environment พร้อม validate | `feat(api): add environment config loading` |
| 1.2 | Structured logger ด้วย `log/slog` | `feat(api): add structured logger` |
| 1.3 | HTTP server ด้วย chi พร้อม graceful shutdown | `feat(api): add HTTP server with graceful shutdown` |
| 1.4 | Middleware: request ID | `feat(api): add request ID middleware` |
| 1.5 | Middleware: access log | `feat(api): add access log middleware` |
| 1.6 | Helper สำหรับ JSON response และ error format ตาม [04](04-api-spec.md#2-suggested-error-format) | `feat(api): add JSON response and error helpers` |
| 1.7 | `GET /healthz` | `feat(api): add health endpoint` |

### 1B. Database

| # | งาน | Commit |
|---|---|---|
| 1.8 | ติดตั้ง goose และ make target `migrate-up` / `migrate-down` | `chore(db): add migration tooling` |
| 1.9 | sqlc config และ make target `generate` | `chore(db): add sqlc configuration` |
| 1.10 | pgx connection pool + ping ตอน start | `feat(api): add PostgreSQL connection pool` |
| 1.11 | CI: รัน migration และ integration test กับ PostgreSQL service | `ci: add PostgreSQL service for integration tests` |

### 1C. Authentication

| # | งาน | Commit |
|---|---|---|
| 1.12 | Migration ตาราง `users` | `feat(db): add users table` |
| 1.13 | Password hashing ด้วย argon2id | `feat(api): add argon2id password hashing` |
| 1.14 | Command `create-user` | `feat(api): add create-user command` |
| 1.15 | Migration ตาราง `sessions` | `feat(db): add sessions table` |
| 1.16 | Session store (create, lookup, revoke) | `feat(api): add session store` |
| 1.17 | `POST /auth/login` | `feat(api): add login endpoint` |
| 1.18 | `POST /auth/logout` | `feat(api): add logout endpoint` |
| 1.19 | Middleware ตรวจ session | `feat(api): add session authentication middleware` |
| 1.20 | `GET /auth/me` | `feat(api): add current user endpoint` |
| 1.21 | CSRF protection | `feat(api): add CSRF protection` |
| 1.22 | Rate limit สำหรับ login | `feat(api): add login rate limiting` |

### 1D. Projects and authorization

| # | งาน | Commit |
|---|---|---|
| 1.23 | Migration `projects` และ `project_members` | `feat(db): add projects and members tables` |
| 1.24 | Authorization policy ตาม role (Owner/Admin/Developer/Viewer) | `feat(api): add project role authorization policy` |
| 1.25 | `POST /projects` และ `GET /projects` | `feat(api): add project create and list endpoints` |
| 1.26 | `GET/PATCH/DELETE /projects/{id}` | `feat(api): add project detail, update and delete endpoints` |
| 1.27 | Endpoint สมาชิกของ project | `feat(api): add project member endpoints` |

### 1E. Servers and Agent enrollment

| # | งาน | Commit |
|---|---|---|
| 1.28 | Migration `servers` (มี `project_id`) และ `agents` | `feat(db): add servers and agents tables` |
| 1.29 | Migration `enrollment_tokens` | `feat(db): add enrollment tokens table` |
| 1.30 | `POST /projects/{id}/servers/enrollment-tokens` | `feat(api): add enrollment token endpoint` |
| 1.31 | `POST /agent/enroll` | `feat(api): add agent enroll endpoint` |
| 1.32 | Middleware ตรวจ Agent credential และสถานะ revoke | `feat(api): add agent authentication middleware` |
| 1.33 | `POST /agent/heartbeat` | `feat(api): add agent heartbeat endpoint` |
| 1.34 | คำนวณ online/offline จาก heartbeat timeout | `feat(api): add server connectivity status` |
| 1.35 | `GET /servers`, `GET /projects/{id}/servers`, `GET /servers/{id}` | `feat(api): add server list and detail endpoints` |
| 1.36 | `DELETE /servers/{id}` พร้อม revoke Agent | `feat(api): add server removal and agent revocation` |

### 1F. Container snapshot

| # | งาน | Commit |
|---|---|---|
| 1.37 | Migration `server_containers` | `feat(db): add server containers table` |
| 1.38 | `POST /agent/containers/snapshot` (กันเก่าทับใหม่, clamp `observed_at`) | `feat(api): add agent container snapshot endpoint` |
| 1.39 | `GET /servers/{id}/containers` พร้อม `stale` | `feat(api): add container list endpoint` |

### 1G. Agent job queue

| # | งาน | Commit |
|---|---|---|
| 1.40 | Migration `jobs`, `job_attempts`, `job_log_chunks` | `feat(db): add jobs tables` |
| 1.41 | Enqueue job พร้อม `NOTIFY` | `feat(api): add job enqueue with notify` |
| 1.42 | Claim query ด้วย `SKIP LOCKED` กรองตาม `target_server_id` | `feat(api): add targeted job claim query` |
| 1.43 | `LISTEN` connection และตัวปลุก waiter ใน memory | `feat(api): add job notification listener` |
| 1.44 | `POST /agent/jobs/claim` แบบ long-poll | `feat(api): add agent long-poll claim endpoint` |
| 1.45 | `POST /agent/jobs/{id}/heartbeat` (ต่อ lease) | `feat(api): add agent job lease renewal endpoint` |
| 1.46 | `POST /agent/jobs/{id}/events` | `feat(api): add agent job events endpoint` |
| 1.47 | `POST /agent/jobs/{id}/complete` | `feat(api): add agent job completion endpoint` |
| 1.48 | `GET /jobs/{id}` และ `GET /jobs/{id}/logs` | `feat(api): add job detail and log endpoints` |
| 1.49 | `POST /servers/{id}/containers/{cid}/actions` | `feat(api): add container action endpoint` |
| 1.50 | `POST /servers/{id}/containers/{cid}/logs` | `feat(api): add container log fetch endpoint` |

### 1H. Agent binary

| # | งาน | Commit |
|---|---|---|
| 1.51 | Config ของ Agent และการเก็บ credential (สิทธิ์ไฟล์ 0600) | `feat(agent): add config and credential storage` |
| 1.52 | Command `enroll` | `feat(agent): add enroll command` |
| 1.53 | Heartbeat loop พร้อม jitter | `feat(agent): add heartbeat loop` |
| 1.54 | Docker client wrapper (เฉพาะ operation ใน allowlist) | `feat(agent): add Docker client wrapper` |
| 1.55 | Snapshot reporter | `feat(agent): add container snapshot reporter` |
| 1.56 | Claim loop + lease renewal + backoff | `feat(agent): add job claim loop` |
| 1.57 | Handler `container_action` | `feat(agent): add container action handler` |
| 1.58 | Handler `fetch_logs` | `feat(agent): add log fetch handler` |
| 1.59 | Make target `build` สำหรับ api และ agent | `chore: add build targets for api and agent` |

### 1I. Web dashboard

| # | งาน | Commit |
|---|---|---|
| 1.60 | Scaffold Vite + React + TypeScript + Tailwind ใน `apps/web` | `chore(web): scaffold Vite React app` |
| 1.61 | Vite dev proxy `/api` ไปที่ Go API | `chore(web): add dev proxy to API` |
| 1.62 | Embed `apps/web/dist` เข้า API binary | `feat(api): embed web assets` |
| 1.63 | Serve static assets + SPA fallback + cache headers | `feat(api): serve web UI with SPA fallback` |
| 1.64 | ติดตั้ง HeroUI และ design tokens | `chore(web): add HeroUI and design tokens` |
| 1.65 | TanStack Router พร้อม route-level code splitting | `feat(web): add router with code splitting` |
| 1.66 | API client และ TanStack Query provider | `feat(web): add API client and query provider` |
| 1.67 | CI: lint + typecheck + build ของ web | `ci: add web lint and build workflow` |
| 1.68 | CI: ตรวจ bundle size budget | `ci: add web bundle size check` |
| 1.69 | หน้า Login | `feat(web): add login page` |
| 1.70 | App shell + navigation + route guard | `feat(web): add authenticated app shell` |
| 1.71 | หน้า Projects | `feat(web): add projects page` |
| 1.72 | หน้า Servers | `feat(web): add servers page` |
| 1.73 | Flow เพิ่ม server (สร้าง token + คำสั่งติดตั้ง) | `feat(web): add server enrollment flow` |
| 1.74 | หน้า Server detail + container list + อายุข้อมูล | `feat(web): add server detail page` |
| 1.75 | ปุ่ม Start/Stop/Restart พร้อม confirm และติดตามสถานะ job | `feat(web): add container actions` |
| 1.76 | Log viewer สำหรับ `fetch_logs` | `feat(web): add container log viewer` |

### 1J. Verification

| # | งาน | Commit |
|---|---|---|
| 1.77 | Integration test: claim พร้อมกันไม่ได้ job ซ้ำ | `test(api): add concurrent job claim test` |
| 1.78 | Integration test: Agent ของ server B claim job ของ server A ไม่ได้ | `test(api): add cross-server claim isolation test` |
| 1.79 | Integration test: revoke แล้วถูกปฏิเสธ รวมถึง long-poll ที่ค้างอยู่ | `test(api): add agent revocation test` |
| 1.80 | Integration test: ผู้ใช้ project อื่นเข้าถึง server ไม่ได้ | `test(api): add cross-project server access test` |
| 1.81 | Integration test: Agent กับ Docker daemon ทิ้งได้ | `test(agent): add Docker integration test` |
| 1.82 | วัด latency จากกด action ถึงเห็นสถานะใหม่ และบันทึกผล | `docs: record phase 1 action latency measurement` |
| 1.83 | วัด RAM ของ API (รวม UI) ตอน idle และบันทึกผล | `docs: record phase 1 idle memory measurement` |

### 1K. Added during Phase 1

Found while building and testing the dashboard; not in the original list.

| # | งาน | Commit |
|---|---|---|
| 1.84 | ให้ `enroll` ถาม token แบบไม่ echo | `feat(agent): prompt for enrollment token without echo` |
| 1.85 | ล้าง error 422 จาก server เมื่อแก้ field (ฟอร์มส่งซ้ำได้ทันที) | `fix(web): clear server validation errors when a field changes` |
| 1.86 | หน้าสมาชิกของ project | `feat(web): add project members page` |
| 1.87 | ปุ่มลบ server (revoke Agent) | `feat(web): add server removal` |
| 1.88 | ปิด action/log ขณะ server offline | `feat(web): pause container actions while a server is offline` |

---

## Phase 2 — Git integration and job queue

| # | งาน | Commit |
|---|---|---|
| 2.1 | 🚧 ยืนยันว่าย้ายกลไกเข้ารหัสมาไว้ phase นี้ (ข้อ 7 ที่ค้าง) | `docs: move secret encryption core to phase 2` |
| 2.2 | AES-GCM encryption พร้อม key version | `feat(api): add authenticated secret encryption` |
| 2.3 | Migration `git_connections` | `feat(db): add git connections table` |
| 2.4 | GitHub App connection flow | `feat(api): add GitHub App connection` |
| 2.5 | GitLab project access token connection | `feat(api): add GitLab token connection` |
| 2.6 | `GET /git-connections/{id}/repositories` | `feat(api): add repository listing endpoint` |
| 2.7 | Migration `applications` | `feat(db): add applications table` |
| 2.8 | Application CRUD | `feat(api): add application endpoints` |
| 2.9 | Migration `webhook_events` | `feat(db): add webhook events table` |
| 2.10 | ตรวจ signature ของ GitHub webhook | `feat(api): add GitHub webhook verification` |
| 2.11 | ตรวจ token ของ GitLab webhook | `feat(api): add GitLab webhook verification` |
| 2.12 | Dedup ด้วย delivery ID | `feat(api): add webhook delivery deduplication` |
| 2.13 | จับคู่ branch แล้ว enqueue build job | `feat(api): add push event build trigger` |
| 2.14 | Sweeper คืน job ที่ lease หมดอายุ | `feat(api): add expired lease recovery` |
| 2.15 | Retry policy แยกตาม job type | `feat(api): add per-type job retry policy` |
| 2.16 | Cancel job | `feat(api): add job cancellation` |
| 2.17 | `GET /jobs` แบบแบ่งหน้า | `feat(api): add job history endpoint` |
| 2.18 | SSE `GET /jobs/{id}/events` พร้อม resume | `feat(api): add job events SSE stream` |
| 2.19 | หน้าตั้งค่า Git connection | `feat(web): add git connection settings` |
| 2.20 | Flow สร้าง Application | `feat(web): add application creation flow` |
| 2.21 | หน้า Jobs + live log ผ่าน SSE | `feat(web): add jobs page with live logs` |
| 2.22 | Test: webhook ซ้ำไม่สร้าง build ซ้ำ | `test(api): add duplicate webhook delivery test` |
| 2.23 | Test: lease หมดแล้ว job กลับมาทำต่อได้ | `test(api): add lease expiry recovery test` |

Added during Phase 2:

| # | งาน | Commit |
|---|---|---|
| 2.11b | ลงทะเบียน/ลบ webhook บน GitLab ตาม lifecycle ของ application | `feat(api): register GitLab webhooks for applications` |
| 2.21b | ขยาย CSS budget เป็น 20 KB (เหตุผลใน 08) | `ci: raise web CSS budget to 20 KB` |

---

## Phase 3 — Isolated CI Runner

| # | งาน | Commit |
|---|---|---|
| 3.1 | 🚧 กำหนด Runner protocol (เสนอ: ใช้ protocol เดียวกับ Agent, `worker_kind = runner`) | `docs: define runner protocol` |
| 3.2 | Migration `runners` + คำสั่ง `create-runner`/`delete-runner` (ไม่มี enrollment ผ่านเว็บ ดู 14) | `feat(api): add runner registration` |
| 3.3 | เปิด claim/lease/events/complete ให้ Runner | `feat(api): add runner job endpoints` |
| 3.4 | Migration และ endpoint ของ registry connection | `feat(api): add registry connections` |
| 3.5 | Runner binary + claim loop | `feat(runner): add job claim loop` |
| 3.6 | Workspace ชั่วคราว + cleanup | `feat(runner): add temporary workspace lifecycle` |
| 3.7 | Checkout ตาม commit SHA | `feat(runner): add exact commit checkout` |
| 3.8 | Step รัน test command | `feat(runner): add test step` |
| 3.9 | Build image ด้วย BuildKit | `feat(runner): add BuildKit image build` |
| 3.10 | Push image + เก็บ digest | `feat(runner): add registry push with digest capture` |
| 3.11 | Timeout และ resource limit | `feat(runner): add build timeout and resource limits` |
| 3.12 | ส่ง log พร้อม redaction | `feat(runner): add build log streaming with redaction` |
| 3.13 | `POST /applications/{id}/builds` | `feat(api): add manual build trigger` |
| 3.14 | แสดงสถานะ build ใน UI | `feat(web): add build status view` |
| 3.15 | Runner setup ใน `deploy/runner` | `chore(runner): add isolated runner deployment setup` |
| 3.16 | Test: build repo ตัวอย่างได้ และ test ล้มแล้ว job failed | `test(runner): add sample repository build test` |

Added during Phase 3 (each shipped inside the commit of the item it supports, not as a separate commit):

| # | งาน | อยู่ใน commit ของ |
|---|---|---|
| 3.4b | `applications.registry_connection_id` + `image_repository` และ field ที่ API/UI | 3.4 |
| 3.5b | แยก client/heartbeat/worker loop ที่ Agent กับ Runner ใช้ร่วมกันเป็น `internal/worker` (Agent protocol ไม่เปลี่ยน; error code `agent_error` เปลี่ยนเป็น `worker_error`) | 3.5 |
| 3.7b | Mint credential ตอน claim (GitHub token แบบ read-only เฉพาะ repo, registry password) และ fail attempt ทันทีถ้าทำไม่ได้ | 3.7, 3.10 |
| 3.13b | Permission `builds.run` (developer ขึ้นไป) และ commit lookup ผ่าน provider API | 3.13 |
| 3.14b | แท็บ Registries, หน้า Application, และแก้ layout ของ dialog ยาว (`ApiForm` เป็น `display: contents`) | 3.14 |

---

## Phase 4 — Deployment and releases

| # | งาน | Commit |
|---|---|---|
| 4.1 | 🚧 สรุป model deployment vs release (ข้อ 6 ที่ค้าง) | `docs: define deployment and release model` |
| 4.2 | 🚧 สรุปวิธีจัดการ port และ Compose config (ข้อ 8 ที่ค้าง) | `docs: define port and compose configuration model` |
| 4.3 | Migration + CRUD ของ `environments` | `feat(api): add environment endpoints` |
| 4.4 | Migration `deployments` และ `releases` | `feat(db): add deployments and releases tables` |
| 4.5 | `POST /environments/{id}/deployments` สร้าง `deploy_image` job | `feat(api): add deployment endpoint` |
| 4.6 | Deploy อัตโนมัติหลัง build สำเร็จ | `feat(api): add automatic deploy after build` |
| 4.7 | ส่ง registry credential แบบ scoped ให้ Agent | `feat(api): add scoped registry credentials for agent jobs` |
| 4.8 | Agent สร้าง Compose file จาก config | `feat(agent): add compose file rendering` |
| 4.9 | Handler `deploy_image` | `feat(agent): add deploy image handler` |
| 4.10 | Health check หลัง deploy | `feat(agent): add post-deploy health check` |
| 4.11 | บันทึก release เมื่อ healthy | `feat(api): add release recording` |
| 4.12 | `POST /releases/{id}/rollback` | `feat(api): add release rollback` |
| 4.13 | หน้า Application detail | `feat(web): add application detail page` |
| 4.14 | หน้า Deployment แสดง stage | `feat(web): add deployment stage view` |
| 4.15 | Release history + ปุ่ม rollback | `feat(web): add release history with rollback` |
| 4.16 | Error UX ของ deployment ที่ล้มเหลว | `feat(web): add deployment failure details` |
| 4.17 | E2E: golden path ครบ | `test: add golden path end-to-end test` |
| 4.18 | เอกสารความเสี่ยงของ persistent volume และ DB migration | `docs: document volume and migration rollback risks` |

Added during Phase 4 (each shipped inside the commit of the item it supports, except the last):

| # | งาน | อยู่ใน commit ของ |
|---|---|---|
| 4.2b | แพ็กเกจ `internal/servicecfg` (config ของ service + validation ที่ Control Plane และ Agent ใช้ร่วมกัน) | 4.3 |
| 4.5b | `jobs.EnqueueIn` (enqueue ใน transaction ของผู้เรียก) และ `jobs.OnSucceeded` (hook ที่รันใน transaction เดียวกับการบันทึกผล job); permission `deployments.run` (developer ขึ้นไป); `deploy_image` retry 2 ครั้ง | 4.5 |
| 4.6b | field `trigger` ใน build payload (`push`/`manual`/`commit`) และ guard กัน build เก่ามาทับ build ใหม่ | 4.6 |
| 4.8b | ย้าย `BuildResult` ไป `agentproto`; แยก `internal/procexec` (ตัวรันคำสั่งที่ Runner กับ Agent ใช้ร่วมกัน), `worker.NewRedactingLog`, และตัวช่วยเทสต์ `internal/dockertest`, `internal/gittest` | 4.9, 4.17 |
| 4.9b | ตัด label ต่อ deployment ออกจาก Compose file (ทำให้ container ถูก restart ทุกครั้งที่ deploy ซ้ำ ซึ่งเทสต์กับ Docker จริงจับได้) | 4.9 |
| 4.11b | `GET /environments/{id}/releases`, `stage` ของ deployment (จาก log ที่ Agent แท็ก `[pull]`…) และ `release_id` | 4.11 |
| 4.12b | ตรวจ port ชนกับ environment อื่นก่อน rollback (`409 port_in_use`) | 4.12 |
| 4.17b | แก้บั๊ก UI ที่พบตอนตรวจในเบราว์เซอร์: "Running now" และ Release history ค้างข้อมูลเก่าหลัง deployment สำเร็จ | `fix(web): refresh releases when a deployment finishes` |

---

## Phase 5 — Production hardening

| # | งาน | Commit |
|---|---|---|
| 5.1 | Project secrets CRUD | `feat(api): add project secret endpoints` |
| 5.2 | Inject secret ตอน deploy | `feat(agent): add secret injection for deployments` |
| 5.3 | Redact ค่า secret ของ project ใน log | `feat(api): add project secret log redaction` |
| 5.4 | Migration `audit_logs` + writer | `feat(api): add audit log writer` |
| 5.5 | บันทึก audit สำหรับ action สำคัญ | `feat(api): record audit events for key actions` |
| 5.6 | หน้า Audit log | `feat(web): add audit log page` |
| 5.7 | ทบทวน RBAC ให้ครบทุก endpoint | `fix(api): enforce role checks on all project resources` |
| 5.8 | Security headers และ CSP | `feat(api): add security headers` |
| 5.9 | Rate limit สำหรับ enrollment และ endpoint สำคัญ | `feat(api): add rate limiting for sensitive endpoints` |
| 5.10 | หมุนเวียน credential ของ Agent | `feat(api): add agent credential rotation` |
| 5.11 | งานลบข้อมูลตาม retention | `feat(api): add retention cleanup` |
| 5.12 | Script และคู่มือ backup/restore | `feat: add backup and restore scripts` |
| 5.13 | Production compose พร้อม TLS | `chore: add production compose with TLS` |
| 5.14 | Script ติดตั้ง Agent + systemd unit | `feat(agent): add install script and systemd unit` |
| 5.15 | Setup Wizard | `feat(web): add setup wizard` |
| 5.16 | คู่มือ upgrade | `docs: add upgrade guide` |

---

## Phase 6 — Usability and performance

| # | งาน | Commit |
|---|---|---|
| 6.1 | หน้า Overview | `feat(web): add overview page` |
| 6.2 | Loading/empty/error/stale state ครบทุกหน้า | `feat(web): complete page state handling` |
| 6.3 | Pagination และ filter ทุก list | `feat(web): add pagination and filters to lists` |
| 6.4 | Virtualize log ยาว | `feat(web): virtualize long logs` |
| 6.5 | ปิด SSE เมื่อออกจากหน้า | `fix(web): close SSE streams on navigation` |
| 6.6 | Accessibility และ responsive | `feat(web): improve accessibility and responsive layout` |
| 6.7 | Benchmark ตาม test profile และบันทึกผล | `docs: add performance benchmark results` |
