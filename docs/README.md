# ForgeDeck

> Self-hosted DevOps Control Plane for Docker-based applications

ForgeDeck คือแพลตฟอร์ม DevOps แบบ Self-hosted สำหรับจัดการ Application, Docker Server, Git Repository, CI/CD Pipeline และ Deployment จาก Dashboard เดียว

เป้าหมายหลักคือทำให้การ Deploy Application ที่ใช้ Docker เป็นเรื่องง่ายสำหรับ Developer โดยไม่จำเป็นต้องเข้า SSH ไปจัดการ Server ด้วยตัวเองทุกครั้ง

---

# 1. Project Overview

## 1.1 Problem

ปัจจุบันการ Deploy Application แบบ Docker มักต้องใช้หลายเครื่องมือร่วมกัน เช่น

- GitHub / GitLab
- GitHub Actions / GitLab CI
- Docker
- Docker Compose
- SSH
- Registry
- Server monitoring
- Log management

Developer ต้องสลับระหว่างหลายระบบ และบางครั้งต้อง SSH เข้า Server เพื่อทำงานที่ควรทำผ่าน Dashboard ได้

ForgeDeck จึงถูกออกแบบมาเพื่อรวม workflow เหล่านี้ไว้ในระบบเดียว

```text
GitHub / GitLab
       │
       │ Webhook
       ▼
┌─────────────────────┐
│      ForgeDeck      │
│    Control Plane    │
│                     │
│ Projects            │
│ Applications        │
│ CI/CD               │
│ Deployments         │
│ Servers             │
│ Secrets             │
│ Audit Logs           │
└─────────┬───────────┘
          │
     ┌────┴─────┐
     ▼          ▼
 Runner       Agent
     │          │
     ▼          ▼
 Build       Production
 Test        Docker Server
 Image       │
     │       ├── App A
     │       ├── App B
     │       └── App C
     ▼
 Registry
```

---

# 2. Product Vision

ForgeDeck มีเป้าหมายเป็น:

> "A lightweight self-hosted DevOps control plane for Docker."

ไม่ใช่ Kubernetes platform

ไม่ใช่ Git hosting

ไม่ใช่ cloud provider

ไม่ใช่ general-purpose CI engine

แต่เป็นระบบที่ทำให้:

```text
Code
 ↓
Build
 ↓
Test
 ↓
Image
 ↓
Registry
 ↓
Deploy
 ↓
Health Check
 ↓
Release
```

ทำได้จาก Dashboard เดียว

---

# 3. Core Principles

## 3.1 Simple

ผู้ใช้ไม่ควรต้องเข้าใจ Infrastructure ทั้งหมดเพื่อ Deploy Application

## 3.2 Lightweight

ไม่ควรมี Dependency ที่กิน Resource โดยไม่จำเป็น

MVP ไม่ใช้:

- Kubernetes
- Kafka
- Redis
- Temporal
- Elasticsearch
- Prometheus
- Grafana

เว้นแต่ในอนาคตจะมีเหตุผลที่ชัดเจน

## 3.3 Fast

Dashboard ต้องตอบสนองเร็ว

หลีกเลี่ยง:

- polling เยอะเกินไป
- animation เยอะเกินไป
- query database ที่ไม่จำเป็น
- loading data ทั้งระบบทุกครั้ง

ใช้:

- Server-side pagination
- SSE
- caching เฉพาะจุด
- incremental loading

## 3.4 Secure by Default

Production Docker Engine ไม่ควรถูกเปิดเป็น Public API

Agent เป็นตัวกลางในการควบคุม Docker

## 3.5 Developer Friendly

UX ต้องเข้าใจง่าย

ตัวอย่าง:

```text
Deploy
  ↓
Queued
  ↓
Building
  ↓
Pushing Image
  ↓
Deploying
  ↓
Health Check
  ↓
Healthy
```

ผู้ใช้ควรรู้ทันทีว่า Deploy อยู่ขั้นไหน

---

# 4. Target Users

## Developer

สามารถ:

- Connect GitHub/GitLab
- สร้าง Application
- Deploy
- ดู Logs
- Restart
- Rollback
- ดู Deployment History

## Team / Organization

สามารถ:

- เพิ่มสมาชิก
- กำหนด Permission
- จัดการหลาย Project
- จัดการหลาย Server

## Server Administrator

สามารถ:

- เพิ่ม Server
- ดู Server Status
- ดู Docker Containers
- ดู Resource Usage
- จัดการ Agent
- Revoke Server

---

# 5. Deployment Model

ForgeDeck เป็น Self-hosted

ผู้ใช้แต่ละคนสามารถติดตั้ง ForgeDeck แยกกันได้

ตัวอย่าง:

```text
Company A
└── ForgeDeck Instance
    ├── PostgreSQL
    ├── Control Plane
    ├── Runner
    └── Agents

Company B
└── ForgeDeck Instance
    ├── PostgreSQL
    ├── Control Plane
    ├── Runner
    └── Agents
```

ไม่จำเป็นต้องมี Central SaaS

---

# 6. High-Level Architecture

ระบบแบ่งออกเป็น 4 ส่วนหลัก

```text
                    ┌───────────────────┐
                    │   Next.js Web UI  │
                    │     HeroUI        │
                    └─────────┬─────────┘
                              │
                              ▼
                    ┌───────────────────┐
                    │   Go Control      │
                    │      Plane        │
                    ├───────────────────┤
                    │ Auth              │
                    │ Projects          │
                    │ Applications      │
                    │ Git Integration   │
                    │ Jobs              │
                    │ Deployments       │
                    │ Secrets           │
                    │ Audit             │
                    └───────┬─────┬─────┘
                            │     │
                  ┌─────────┘     └──────────┐
                  ▼                          ▼
          ┌──────────────┐           ┌──────────────┐
          │ PostgreSQL   │           │ CI Runner    │
          └──────────────┘           └──────┬───────┘
                                            │
                                            ▼
                                      ┌────────────┐
                                      │  Registry  │
                                      └─────┬──────┘
                                            │
                                            ▼
                                     ┌─────────────┐
                                     │ Docker Agent│
                                     └──────┬──────┘
                                            │
                                            ▼
                                      Production
                                        Server
```

---

# 7. Components

## 7.1 Control Plane

เขียนด้วย Go

เป็นหัวใจหลักของระบบ

รับผิดชอบ:

- Authentication
- Authorization
- Projects
- Applications
- Git integrations
- Webhooks
- Job scheduling
- CI/CD orchestration
- Deployment management
- Server management
- Agent management
- Secrets
- Audit logs

Control Plane ควรเป็น Modular Monolith ในช่วงแรก

ไม่ควรแยกเป็น Microservices จำนวนมากตั้งแต่แรก

---

# 8. Frontend

## Stack

- Next.js
- TypeScript
- Tailwind CSS
- HeroUI
- TanStack Query

## UI Philosophy

ต้องการ UI ที่:

- Clean
- Minimal
- Friendly
- Modern
- Fast
- ไม่รก
- ไม่เต็มไปด้วย Dashboard Card
- ไม่ทำให้ผู้ใช้รู้สึกว่าเป็น Enterprise software ที่ซับซ้อนเกินไป

---

# 9. Main Dashboard

Dashboard แสดงข้อมูลสำคัญเท่านั้น

ตัวอย่าง:

```text
Overview

Servers
┌──────────────┐
│ 3 Online     │
└──────────────┘

Applications
┌──────────────┐
│ 12 Running   │
└──────────────┘

Recent Deployments
──────────────────────────────
my-api       Production   ✓
frontend     Production   ✓
worker       Staging      ✗
──────────────────────────────

Recent Activity
──────────────────────────────
Deployment completed
Server connected
Build failed
──────────────────────────────
```

ไม่ควรใส่ Graph ใหญ่ ๆ ทุกหน้า

---

# 10. Application

Application คือ Software ที่ต้องการ Deploy

ตัวอย่าง:

```text
Project: My Startup

Applications

├── API
├── Frontend
├── Worker
└── Cron
```

แต่ละ Application สามารถกำหนด:

- Git repository
- Branch
- Dockerfile
- Build context
- Environment
- Registry
- Deployment strategy
- Health check
- Resource limits

---

# 11. Environment

Application หนึ่งตัวสามารถมีหลาย Environment

ตัวอย่าง:

```text
API

├── Development
├── Staging
└── Production
```

แต่ละ Environment สามารถอยู่คนละ Server ได้

```text
Development
    ↓
Dev Server

Staging
    ↓
Staging Server

Production
    ↓
Production Server
```

---

# 12. Server

Server คือเครื่องที่ติดตั้ง ForgeDeck Agent

ตัวอย่าง:

```text
Production Server 01

Status: Online

CPU      23%
Memory   51%
Disk     42%

Docker

├── api
├── frontend
├── postgres
└── nginx
```

Server ไม่จำเป็นต้องเปิด Port Docker API ออก Internet

---

# 13. Agent

Agent เป็นโปรแกรมขนาดเล็กที่ติดตั้งบน Production Server

หน้าที่:

- ติดต่อ Control Plane
- Heartbeat
- รับ Deployment Job
- ควบคุม Docker Engine
- ดึง Image
- Start Container
- Stop Container
- Restart Container
- ดู Logs
- Health Check
- ส่งสถานะกลับ Control Plane

---

# 14. Agent Communication

ใช้ Pull Model

ไม่ให้ Control Plane เปิด connection เข้า Server

```text
Agent
   │
   │ HTTPS
   ▼
Control Plane
```

Agent จะ:

```text
Heartbeat
   ↓
Check Job
   ↓
Claim Job
   ↓
Execute
   ↓
Send Logs
   ↓
Complete
```

ข้อดี:

- Server อยู่หลัง NAT ได้
- ไม่ต้องเปิด inbound port
- Firewall configuration ง่าย
- Security ดีกว่าเปิด Docker TCP

---

# 15. Agent Heartbeat

Agent ส่ง heartbeat ทุกประมาณ 15–30 วินาที

ตัวอย่าง:

```json
{
  "agent_id": "agent_123",
  "status": "online",
  "version": "0.1.0",
  "cpu": 21,
  "memory": 48,
  "disk": 42
}
```

ถ้าไม่ heartbeat เกิน threshold:

```text
Online
   ↓
Unreachable
   ↓
Offline
```

---

# 16. Runner

Runner แยกออกจาก Production

Runner มีหน้าที่:

- Clone repository
- Checkout commit
- Install dependencies
- Run tests
- Build Docker image
- Push image
- ส่ง logs

ตัวอย่าง:

```text
GitHub
   ↓
Runner
   ↓
git checkout abc123
   ↓
npm test
   ↓
docker build
   ↓
docker push
   ↓
Registry
```

Runner ห้ามควบคุม Production Docker Engine โดยตรง

---

# 17. Why Separate Runner and Production?

ไม่ควร Build บน Production Server

เพราะ:

- Build อาจกิน CPU
- Build อาจกิน RAM
- Build อาจกิน Disk
- Build ที่ไม่ปลอดภัยอาจกระทบ Production
- CI workload ไม่ควรทำให้ Production application ล่ม

ดังนั้น:

```text
Runner
    ↓
Build

Production
    ↓
Run
```

แยกหน้าที่กันชัดเจน

---

# 18. Build Isolation

Runner ต้องถือว่า Source Code เป็น Untrusted Input

ไม่ควรทำ:

```text
docker run ...
-v /var/run/docker.sock:/var/run/docker.sock
```

กับ build workload ที่ไม่ trusted

เพราะ Docker Socket สามารถนำไปควบคุม Host ได้

แนวทาง MVP:

- Dedicated Runner
- Resource limit
- Build timeout
- Temporary workspace
- BuildKit / Buildx
- จำกัด Network ตามความเหมาะสม
- ลบ workspace หลัง Build

---

# 19. Registry

Image ที่ Build สำเร็จจะถูก Push ไป Registry

รองรับ:

- GHCR
- GitLab Container Registry
- Self-hosted Registry

ตัวอย่าง:

```text
ghcr.io/company/my-api:abc123
```

ควร Deploy ด้วย Image Digest มากกว่า Tag

เช่น:

```text
sha256:abc123...
```

เพราะ Digest เป็น Immutable

---

# 20. CI/CD Flow

เมื่อ Developer Push Code:

```text
Developer
    │
    ▼
GitHub / GitLab
    │
    │ Webhook
    ▼
ForgeDeck
    │
    ▼
Create Job
    │
    ▼
Runner
    │
    ├── Checkout
    ├── Test
    ├── Build
    └── Push
            │
            ▼
         Registry
            │
            ▼
        Deployment
            │
            ▼
         Agent
            │
            ▼
      Docker Server
            │
            ▼
       Health Check
            │
            ▼
         Healthy
```

---

# 21. Deployment Strategy

MVP ใช้ Docker Compose / Docker Engine

ไม่ใช้ Kubernetes

ตัวอย่าง:

```yaml
services:
  api:
    image: ghcr.io/example/api@sha256:...
    restart: unless-stopped
    ports:
      - "8080:8080"
```

ForgeDeck สามารถสร้าง/แก้ไข Deployment Configuration ให้ Agent ดำเนินการ

---

# 22. Deployment State

Deployment ต้องมี State ที่ชัดเจน

```text
QUEUED
  ↓
BUILDING
  ↓
PUSHING_IMAGE
  ↓
DEPLOYING
  ↓
HEALTH_CHECK
  ↓
HEALTHY
```

กรณีผิดพลาด:

```text
FAILED
```

หรือ

```text
CANCELLED
```

---

# 23. Release

ทุก Deployment ที่สำเร็จควรสร้าง Release

ตัวอย่าง:

```text
Release #42

Commit:
abc123

Image:
ghcr.io/company/api

Digest:
sha256:...

Environment:
Production

Status:
Healthy
```

ทำให้สามารถย้อนกลับไป Release ก่อนหน้าได้

---

# 24. Rollback

Rollback ไม่ควร Build ใหม่

ควรใช้ Image เดิม

ตัวอย่าง:

```text
Release #43
    ↓
Problem
    ↓
Rollback
    ↓
Release #42
```

เพราะ Image Digest ของ Release #42 ยังอยู่

---

# 25. Health Check

หลัง Deploy ต้องตรวจสอบว่า Application ทำงานจริง

ตัวอย่าง:

```text
GET /health

200 OK
```

สามารถกำหนด:

```text
Health Check URL
/health

Timeout
30 sec

Retries
3
```

Flow:

```text
Deploy
 ↓
Container Running?
 ↓
Health Endpoint?
 ↓
HTTP 200?
 ↓
Healthy
```

ถ้าไม่ผ่าน:

```text
Deployment Failed
```

และในอนาคตสามารถ Auto Rollback ได้

---

# 26. Job System

PostgreSQL จะทำหน้าที่เป็น Job Queue ใน MVP

ไม่จำเป็นต้องใช้ Redis/Kafka ตั้งแต่แรก

Job:

```text
queued
running
succeeded
failed
cancelled
```

แต่ละ Job มี:

- ID
- Type
- Status
- Payload
- Attempt count
- Lease expiration
- Created time
- Started time
- Finished time

---

# 27. Job Lease

เพื่อป้องกัน Runner/Agent ตายระหว่างทำงาน

ตัวอย่าง:

```text
Job
 ↓
Claim
 ↓
lease_expires_at
```

Agent ต้องต่อ lease

ถ้า Agent หาย:

```text
lease expired
      ↓
Job กลับมา queued
      ↓
Retry
```

---

# 28. Retry

ควรออกแบบให้ Job รองรับ Retry

แต่ต้องระวัง Duplicate Execution

ดังนั้น Job ต้องมี:

```text
idempotency_key
```

Deployment operation ควรออกแบบให้ทำซ้ำแล้วไม่ทำให้ระบบเสียหาย

---

# 29. Logs

Job logs ไม่ควรใช้ polling หนัก ๆ

ใช้ SSE:

```text
Browser
   │
   │ SSE
   ▼
Control Plane
   │
   ▼
Job Logs
```

หน้า Deployment สามารถแสดง:

```text
Building...

Step 1/8
Installing dependencies...

Step 2/8
Running tests...

✓ Tests passed

Building Docker image...

████████████████ 100%

Deploying...
```

---

# 30. Git Integration

รองรับ:

## GitHub

ใช้ GitHub App ในอนาคต

ข้อดี:

- Short-lived token
- จำกัด Repository
- จำกัด Permission

## GitLab

รองรับ:

- GitLab OAuth
- Project Access Token
- Webhook

---

# 31. Webhook

Endpoint:

```text
POST /webhooks/github
POST /webhooks/gitlab
```

ต้องตรวจสอบ:

- Signature
- Secret
- Event type
- Repository
- Branch
- Commit SHA

ต้องมี webhook event deduplication

เพราะ Git Provider อาจส่ง Event ซ้ำ

---

# 32. Authentication

MVP สามารถใช้:

- Email + Password
- Session Cookie

ในอนาคต:

- GitHub Login
- GitLab Login
- Google Login
- OIDC

Password ต้องเก็บเป็น Hash

ห้ามเก็บ Plain Text

---

# 33. Authorization

เริ่มด้วย RBAC

ตัวอย่าง:

```text
Owner
Admin
Developer
Viewer
```

ตัวอย่าง Permission:

```text
Viewer
  └── View

Developer
  ├── View
  ├── Deploy
  └── Restart

Admin
  ├── Manage Application
  ├── Manage Server
  └── Manage Members

Owner
  └── Everything
```

---

# 34. Secrets

Secret เช่น:

```text
DATABASE_URL
API_KEY
STRIPE_SECRET_KEY
```

ไม่ควรเก็บ Plain Text

ควร:

```text
User
 ↓
Control Plane
 ↓
Encrypt
 ↓
PostgreSQL
```

Encryption key ต้องอยู่นอก Database

เช่น Environment Variable / external secret source

---

# 35. Secret Redaction

Logs ต้องพยายามไม่แสดง Secret

ตัวอย่าง:

```text
API_KEY=********
```

ไม่ควร:

```text
API_KEY=sk_live_xxxxxxxxx
```

---

# 36. Audit Log

ทุก Action สำคัญควรบันทึก

ตัวอย่าง:

```text
User
jakkayy

Action
deployment.create

Application
my-api

Environment
production

Time
2026-10-13 01:20
```

ตัวอย่าง Action:

- login
- logout
- create project
- delete application
- deploy
- rollback
- restart
- add server
- revoke agent
- update secret

---

# 37. Database

ใช้ PostgreSQL

Core tables:

```text
users
projects
project_members

git_connections

applications
environments

servers
agents

pipelines
jobs
job_attempts
job_log_chunks

releases

project_secrets

webhook_events

audit_logs
```

---

# 38. Database Relationship

```text
User
 │
 └── Project
       │
       ├── Members
       │
       ├── Git Connection
       │
       └── Applications
              │
              └── Environments
                     │
                     ├── Server
                     │
                     └── Releases
                            │
                            └── Jobs
```

---

# 39. Important Database Concepts

ทุก Entity สำคัญควรใช้ UUID

เช่น:

```text
project_id
application_id
server_id
job_id
release_id
```

Job ต้องมี:

```text
status
attempt_count
lease_expires_at
idempotency_key
```

Release ต้องเก็บ:

```text
commit_sha
image
image_digest
environment_id
status
```

---

# 40. Backend Stack

แนะนำ:

```text
Go
Chi
pgx
sqlc
PostgreSQL
SSE
Docker Engine API
```

Go Backend เป็น Modular Monolith

---

# 41. Backend Structure

```text
cmd/
├── api/
├── runner/
└── agent/

internal/
├── auth/
├── projects/
├── applications/
├── environments/
├── gitprovider/
├── pipelines/
├── jobs/
├── deployments/
├── docker/
├── agentprotocol/
├── secrets/
└── audit/

migrations/

pkg/

docs/
```

ไม่จำเป็นต้องแยก:

```text
auth-service
project-service
deployment-service
job-service
```

ตั้งแต่แรก

---

# 42. API Design

Base:

```text
/api/v1
```

Authentication:

```text
POST /auth/login
POST /auth/logout
GET  /auth/me
```

Projects:

```text
GET    /projects
POST   /projects
GET    /projects/:id
DELETE /projects/:id
```

Applications:

```text
GET    /projects/:id/applications
POST   /projects/:id/applications
GET    /applications/:id
PATCH  /applications/:id
DELETE /applications/:id
```

Servers:

```text
GET  /servers
POST /servers
POST /servers/enrollment-tokens
GET  /servers/:id
DELETE /servers/:id
```

Containers:

```text
GET  /servers/:id/containers
POST /containers/:id/actions
```

Jobs:

```text
GET /jobs/:id
GET /jobs/:id/events
POST /jobs/:id/cancel
```

Deployments:

```text
POST /environments/:id/deployments
GET  /environments/:id/deployments
```

Releases:

```text
GET  /environments/:id/releases
POST /releases/:id/rollback
```

Webhooks:

```text
POST /webhooks/github
POST /webhooks/gitlab
```

---

# 43. Agent API

```text
POST /agent/enroll

POST /agent/heartbeat

POST /agent/jobs/claim

POST /agent/jobs/:id/heartbeat

POST /agent/jobs/:id/events

POST /agent/jobs/:id/complete
```

Agent ต้องมี Identity ของตัวเอง

สามารถ Revoke ได้จาก Dashboard

---

# 44. Frontend Pages

```text
/login

/dashboard

/projects
/projects/:id

/applications
/applications/:id

/applications/:id/deployments

/applications/:id/logs

/servers
/servers/:id

/jobs

/settings

/settings/members
/settings/git
/settings/security
/settings/audit
```

---

# 45. Application Page

ตัวอย่าง:

```text
My API

Production
● Healthy

Repository
github.com/company/my-api

Branch
main

Current Release
#42

Commit
abc123

Actions

[Deploy] [Restart] [Logs] [Settings]
```

ด้านล่าง:

```text
Deployment History

#42   abc123   Healthy
#41   91bc21   Healthy
#40   123abc   Failed
```

---

# 46. Deployment UI

หน้า Deployment ควรเน้น State

```text
Deploy #42

✓ Queued
✓ Building
✓ Pushing Image
✓ Deploying
● Health Check

--------------------------------

Logs

Waiting for health check...

GET /health
200 OK

Application is healthy.
```

---

# 47. Error UX

Error ไม่ควรแสดงเพียง:

```text
Deployment failed.
```

ควรแสดง:

```text
Deployment failed

Health check failed after 3 attempts.

Endpoint:
GET /health

Last response:
503 Service Unavailable

[View Logs]
[Retry]
[Rollback]
```

---

# 48. Performance Strategy

Frontend:

- TanStack Query
- Pagination
- Lazy loading
- SSE เฉพาะหน้าที่ต้องใช้
- ไม่ polling ทุกอย่าง
- จำกัด animation
- Virtualization สำหรับ log ยาว
- โหลดข้อมูลเฉพาะหน้าปัจจุบัน

Backend:

- Stateless API
- PostgreSQL indexes
- Connection pooling
- SSE
- Batch operations
- ไม่ query Docker stats ถี่เกินไป

Agent:

- Heartbeat 15–30 sec
- ส่ง resource metrics เป็นช่วง
- ส่ง logs แบบ stream/chunk

---

# 49. Docker Management

Agent สามารถจัดการ:

```text
List Containers
Start
Stop
Restart
Remove
Logs
Inspect
Stats
```

แต่ไม่ควรเปิดเป็น:

```text
Web Shell
```

ใน MVP

เพราะ Security Risk สูงมาก

---

# 50. Security Threat Model

ต้องระวัง:

## Docker Socket

ห้าม expose:

```text
tcp://0.0.0.0:2375
```

## Runner Escape

Build workload ต้อง isolate

## SSRF

หาก user กำหนด URL เอง ต้องป้องกัน:

```text
http://localhost
http://127.0.0.1
http://169.254.169.254
```

## Webhook Forgery

ต้องตรวจ Signature

## Credential Leakage

ห้ามใส่ Secret ใน Logs

## Session Security

ใช้ Secure Cookie และ HttpOnly

## Agent Authentication

Agent ทุกตัวต้องมี credential ของตัวเอง

---

# 51. Project Structure

```text
forgedeck/
│
├── apps/
│   └── web/
│
├── cmd/
│   ├── api/
│   ├── runner/
│   └── agent/
│
├── internal/
│   ├── auth/
│   ├── projects/
│   ├── applications/
│   ├── environments/
│   ├── gitprovider/
│   ├── pipelines/
│   ├── jobs/
│   ├── deployments/
│   ├── docker/
│   ├── agentprotocol/
│   ├── secrets/
│   └── audit/
│
├── migrations/
│
├── packages/
│   └── api-client/
│
├── deploy/
│   ├── compose/
│   └── runner/
│
├── docs/
│   ├── architecture.md
│   ├── api.md
│   ├── agent-protocol.md
│   ├── security.md
│   ├── deployment.md
│   └── development.md
│
├── docker-compose.yml
├── Makefile
└── README.md
```

---

# 52. Development Environment

Local development:

```text
Next.js
Go API
PostgreSQL
Docker
```

สามารถใช้ Docker Compose:

```text
docker compose up -d
```

ตัวอย่าง:

```text
web
api
postgres
```

Runner และ Agent สามารถ Run แยกตอน Development

---

# 53. Production Installation

เป้าหมายคือให้ติดตั้งง่าย

ตัวอย่างในอนาคต:

```bash
curl -fsSL https://get.forgedeck.dev | sh
```

หรือ:

```bash
docker compose up -d
```

ควรมี Setup Wizard:

```text
Welcome to ForgeDeck

[ Start Setup ]

1. Create Admin
2. Database
3. Git Provider
4. Runner
5. First Server
6. Finish
```

---

# 54. Initial Installation Architecture

```text
ForgeDeck Server

├── web
├── api
├── postgres
└── runner
```

Production Server:

```text
Production Server

└── ForgeDeck Agent
```

---

# 55. MVP Scope

MVP ต้องมีเพียง:

### Authentication

- Login
- Logout
- User

### Project

- Create project
- Project members

### Git

- GitHub
- GitLab
- Repository selection
- Webhook

### Server

- Add server
- Agent enrollment
- Heartbeat
- Docker containers

### CI

- Checkout
- Test
- Docker build
- Registry push

### CD

- Deploy
- Health check
- Release
- Rollback

### UI

- Dashboard
- Applications
- Servers
- Deployments
- Logs

---

# 56. สิ่งที่ไม่ทำใน MVP

ห้าม Scope บาน

ไม่ทำ:

- Kubernetes
- Kubernetes cluster management
- Own Git hosting
- Cloud provisioning
- AWS infrastructure management
- Terraform platform
- Arbitrary web terminal
- General-purpose CI YAML engine
- Multi-region deployment
- Service mesh
- Kafka
- Redis
- Temporal
- Elasticsearch
- Advanced observability platform

---

# 57. Development Roadmap

## Phase 1 — Foundation

เป้าหมาย:

> ทำให้ ForgeDeck สามารถจัดการ Docker Server ได้

ทำ:

```text
Go API
PostgreSQL
Authentication
Projects
Servers
Agent
Heartbeat
Docker containers
Logs
Start/Stop/Restart
```

ผลลัพธ์:

```text
Dashboard
   ↓
Server
   ↓
Docker
```

---

# 58. Phase 2 — Git + Job System

เพิ่ม:

```text
GitHub
GitLab
Webhook
Jobs
Job Queue
Lease
Retry
SSE Logs
```

Flow:

```text
Push Code
 ↓
Webhook
 ↓
Job
 ↓
Runner
```

---

# 59. Phase 3 — CI Runner

เพิ่ม:

```text
Git Checkout
Tests
Docker Build
BuildKit
Registry
Image Digest
```

Flow:

```text
Git
 ↓
Runner
 ↓
Docker Image
 ↓
Registry
```

---

# 60. Phase 4 — Deployment

เพิ่ม:

```text
Environment
Release
Deployment
Health Check
Rollback
```

Flow:

```text
Image
 ↓
Agent
 ↓
Docker
 ↓
Health Check
 ↓
Release
```

---

# 61. Phase 5 — Production Hardening

เพิ่ม:

```text
Secret Encryption
Audit Logs
RBAC
Security Hardening
Backup
Restore
Upgrade
Install Wizard
TLS
```

---

# 62. Future Features

หลัง MVP สำเร็จสามารถเพิ่ม:

- GitHub OAuth
- GitLab OAuth
- Google OIDC
- Team/Organization
- Approval deployment
- Scheduled deployment
- Auto rollback
- Blue/Green deployment
- Canary deployment
- Domain management
- HTTPS automation
- Environment variables
- Database backup
- Server metrics
- Notification
- Slack
- Discord
- LINE
- Email
- Plugin system

---

# 63. Long-term Architecture

ในอนาคตอาจขยายเป็น:

```text
                    ForgeDeck
                       │
          ┌────────────┼────────────┐
          │            │            │
       GitHub        GitLab       Registry
          │            │            │
          └────────────┼────────────┘
                       │
                Control Plane
                       │
        ┌──────────────┼──────────────┐
        │              │              │
      Runner         Agent          Agent
        │              │              │
        ▼              ▼              ▼
     Build         Server A        Server B
                       │
                 ┌─────┴─────┐
                 │           │
                API       Frontend
```

แต่ยังคงแนวคิด:

> Simple until complexity is actually needed.

---

# 64. Suggested Repository Strategy

เริ่มต้นเป็น Monorepo:

```text
forgedeck/

apps/web

cmd/api
cmd/runner
cmd/agent

internal/...

docs/...
```

ข้อดี:

- แชร์ Type/API definitions ง่าย
- CI ง่าย
- Development ง่าย
- Versioning ง่าย

---

# 65. Testing Strategy

## Backend

Unit test:

```text
jobs
deployments
auth
webhooks
secrets
```

Integration test:

```text
PostgreSQL
Docker
Agent protocol
```

## Frontend

Test:

```text
Login
Create Application
Deploy
View Logs
Rollback
```

## E2E

Flow สำคัญที่สุด:

```text
Create Project
 ↓
Connect GitHub
 ↓
Create Application
 ↓
Push Code
 ↓
Build
 ↓
Push Image
 ↓
Deploy
 ↓
Health Check
 ↓
Healthy
```

---

# 66. Observability

MVP ไม่จำเป็นต้องมี Prometheus/Grafana

เริ่มด้วย:

```text
Application Logs
Job Logs
Audit Logs
Server Status
Health Status
```

Backend ควรมี structured logging:

```json
{
  "level": "info",
  "event": "deployment.completed",
  "deployment_id": "...",
  "application_id": "...",
  "duration_ms": 12450
}
```

---

# 67. Resource Goals

ForgeDeck ถูกออกแบบให้เหมาะกับเครื่องขนาดเล็ก

ตัวอย่าง Target:

```text
2 CPU
4 GB RAM
```

สามารถรัน:

```text
ForgeDeck
PostgreSQL
Runner
```

ได้ในระดับ Development / Small Production workload

Production workload จริงของ Application จะขึ้นกับ Server ที่นำ Agent ไปติดตั้ง

---

# 68. Why Go?

Go เหมาะกับ Project นี้เพราะ:

- Binary เล็ก
- Memory usage ต่ำ
- Concurrency ดี
- เหมาะกับ Agent
- เหมาะกับ API Server
- Docker ecosystem ดี
- Build เป็น single binary ได้
- Deployment ง่าย

โดยเฉพาะ Agent:

```text
forgedeck-agent
```

สามารถ compile เป็น binary เดียวได้

---

# 69. Why Next.js?

ใช้สำหรับ:

- Dashboard
- Routing
- Authentication UI
- Server-side rendering ที่จำเป็น
- Developer ecosystem
- TypeScript

แต่ Frontend ต้องระวังไม่ทำให้เป็น SPA ที่หนักเกินไป

---

# 70. Why HeroUI?

HeroUI เหมาะกับแนวคิดของ ForgeDeck เพราะต้องการ:

- Modern UI
- Clean components
- Accessible components
- ลดเวลาเขียน UI เอง
- Consistent design

Component หลัก:

```text
Button
Modal
Dropdown
Tabs
Table
Card
Chip
Tooltip
Drawer
Toast
Progress
Skeleton
```

---

# 71. UX Rules

ทุกหน้าควรตอบคำถาม:

> "ตอนนี้ระบบเป็นอะไร?"

ตัวอย่าง:

```text
API
● Healthy
```

ไม่ใช่:

```text
Status: 1
```

และ:

```text
Deploying...
Building image...
```

ควรเข้าใจได้ทันทีโดยไม่ต้องอ่าน Documentation

---

# 72. Design Language

Visual direction:

```text
Minimal
Clean
Developer-focused
Soft
Fast
Professional
```

หลีกเลี่ยง:

```text
Dashboard ที่เต็มไปด้วย Card
กราฟเยอะ
Gradient เยอะ
Animation เยอะ
สีเยอะ
Sidebar ซับซ้อน
```

---

# 73. Core User Journey

User ใหม่:

```text
Login
 ↓
Create Project
 ↓
Connect GitHub
 ↓
Select Repository
 ↓
Create Application
 ↓
Select Server
 ↓
Configure Environment
 ↓
Deploy
 ↓
Watch Build
 ↓
Watch Deployment
 ↓
Healthy
```

เป้าหมายคือทำให้ Flow นี้สั้นและเข้าใจง่ายที่สุด

---

# 74. Golden Path

ForgeDeck ต้องมี Golden Path:

```text
GitHub
   ↓
Dockerfile
   ↓
ForgeDeck
   ↓
Build
   ↓
Registry
   ↓
Server
   ↓
Docker
   ↓
Healthy
```

ถ้า User ทำตาม Default Configuration ก็ควร Deploy ได้โดยไม่ต้อง Configure สิ่งต่าง ๆ จำนวนมาก

---

# 75. Core Differentiator

ForgeDeck ไม่ได้พยายามแข่งด้วยจำนวน Feature

แต่แข่งด้วย:

```text
Simplicity
+
Speed
+
Self-hosted
+
Docker-native
+
Developer UX
```

Positioning:

> Deploy your Docker applications without managing the infrastructure manually.

---

# 76. Final Architecture

```text
                         ┌────────────────────┐
                         │      Browser       │
                         │ Next.js + HeroUI   │
                         └─────────┬──────────┘
                                   │
                                   ▼
                         ┌────────────────────┐
                         │   Go Control Plane │
                         │                    │
                         │ Auth               │
                         │ Projects           │
                         │ Applications       │
                         │ Git                │
                         │ Jobs               │
                         │ Deployments        │
                         │ Secrets            │
                         │ Audit              │
                         └──────┬───────┬─────┘
                                │       │
                         ┌──────┘       └───────┐
                         ▼                       ▼
                  ┌─────────────┐        ┌─────────────┐
                  │ PostgreSQL  │        │    Runner   │
                  └─────────────┘        └──────┬──────┘
                                                 │
                                                 ▼
                                           ┌───────────┐
                                           │ Registry  │
                                           └─────┬─────┘
                                                 │
                                                 ▼
                                          ┌─────────────┐
                                          │    Agent    │
                                          └──────┬──────┘
                                                 │
                                                 ▼
                                      ┌────────────────────┐
                                      │ Production Server  │
                                      │                    │
                                      │ Docker Engine      │
                                      │ ├── API            │
                                      │ ├── Frontend       │
                                      │ ├── Worker         │
                                      │ └── Other Apps     │
                                      └────────────────────┘
```

---

# 77. MVP Definition of Done

ForgeDeck MVP ถือว่าสำเร็จเมื่อ User สามารถ:

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

ถ้า Flow นี้ทำงานได้ครบ:

> ForgeDeck MVP ถือว่าเป็น DevOps Platform ที่ใช้งานได้จริงแล้ว

---

# 78. Development Priority

ลำดับที่แนะนำ:

```text
1. PostgreSQL
2. Go API
3. Authentication
4. Project
5. Server
6. Agent
7. Docker management
8. Frontend Dashboard
9. GitHub/GitLab
10. Webhook
11. Job Queue
12. Runner
13. Registry
14. Deployment
15. Health Check
16. Release
17. Rollback
18. Secrets
19. Audit
20. Security Hardening
```

อย่ากระโดดไปทำ CI/CD ก่อน Agent + Docker Management เสถียร

---

# 79. First Milestone

Milestone แรกควรเป็น:

> "Manage a remote Docker server from the ForgeDeck Dashboard."

เมื่อทำได้:

```text
Browser
 ↓
ForgeDeck
 ↓
Agent
 ↓
Docker
```

แล้วสามารถ:

```text
View Containers
Start
Stop
Restart
View Logs
```

ถือว่า Architecture หลักเริ่มพิสูจน์แล้ว

จากนั้นค่อยต่อ:

```text
Git
 ↓
CI
 ↓
Registry
 ↓
CD
```

---

# 80. Project Philosophy

ForgeDeck ควรยึดหลักนี้ตลอดการพัฒนา:

> Don't build infrastructure for the sake of infrastructure.

ถ้า PostgreSQL ทำงานได้ ก็ไม่ต้องเพิ่ม Redis

ถ้า Docker Compose ทำงานได้ ก็ไม่ต้องเพิ่ม Kubernetes

ถ้า Agent + HTTPS ทำงานได้ ก็ไม่ต้องเพิ่ม Message Broker

ถ้า SSE ทำงานได้ ก็ไม่ต้องเพิ่มระบบ Streaming ที่ซับซ้อน

เป้าหมายไม่ใช่สร้างระบบที่มี Technology เยอะที่สุด

แต่คือสร้างระบบที่:

**ใช้ง่ายที่สุด, เบาที่สุด, และ Deploy ได้จริง**
