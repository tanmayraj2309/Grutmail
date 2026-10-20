# 🚀 GrutMail - High-Throughput Concurrent Email Engine

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-DLQ-DC382D?logo=redis&logoColor=white)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)](https://www.docker.com/)

**GrutMail** is a high-performance, concurrent bulk email dispatching platform built with **Go**, featuring a minimalist monochromatic web dashboard, **PostgreSQL** contact persistence, **Redis Dead Letter Queue (DLQ)** for fault tolerance, and **Mailpit** for local mock SMTP testing.

---

## 🌟 Key Features

- **⚡ Concurrent Worker Pool**: Powered by Go Goroutines, buffered channels, and `sync.WaitGroup` to dispatch thousands of emails concurrently with throttled pacing.
- **🎨 Minimalist Web Dashboard**: Modern monochromatic light-theme interface to monitor delivery metrics, inspect live logs, and manage campaigns.
- **📝 Live Email Template Customizer**: Real-time subject and body editor with dynamic tag tokens (`{{.Name}}`, `{{.Email}}`), instant live preview mockup, and memory hot-reloading.
- **🗄️ PostgreSQL Contact Registry**: Permanent storage and status lifecycle tracking (`PENDING`, `SENT`, `FAILED`) with automated CSV seeding.
- **📫 Redis Dead Letter Queue (DLQ)**: Automatic failure capture for dropped SMTP connections or template parsing issues, complete with one-click re-queueing and retry.
- **🐳 One-Click Docker Setup**: Complete local infrastructure (Mailpit + PostgreSQL + Redis) managed via Docker Compose.

---

## 🏛️ System Architecture

```mermaid
flowchart TD
    A[emails.csv] -->|Auto-seed on first run| B[(PostgreSQL Database)]
    B -->|Fetch PENDING contacts| C[Producer Goroutine]
    C -->|Stream via Buffered Channel| D[5 Concurrent Worker Goroutines]
    
    D -->|1. Render in Memory| E[Pre-parsed email.tmpl]
    D -->|2. SMTP Dispatch| F[Mailpit / Cloud SMTP]
    
    F -->|Success| G[PostgreSQL: status = SENT]
    F -->|Failure / Timeout| H[PostgreSQL: status = FAILED]
    H -->|Push JSON Payload| I[(Redis DLQ: email:dlq)]
    
    J[GrutMail Web UI :8080] <-->|REST API| B
    J <-->|Inspect & Retry| I
    J -->|Trigger Campaign| C
```

---

## 🚀 Quick Start (Local Setup)

### Prerequisites
- [Docker](https://www.docker.com/) & Docker Compose
- [Go (1.22+)](https://golang.org/)

### 1. Clone the repository
```bash
git clone https://github.com/<YOUR_USERNAME>/<YOUR_REPO_NAME>.git
cd <YOUR_REPO_NAME>
```

### 2. Start Infrastructure Services
Start Mailpit, PostgreSQL, and Redis in the background:
```bash
docker compose up -d
```

### 3. Run GrutMail
```bash
go run .
```

### 4. Access the Dashboards
- **GrutMail Web Dashboard**: [http://localhost:8080](http://localhost:8080)
- **Mailpit Real-Time Inbox**: [http://localhost:8025](http://localhost:8025)

---

## ⚙️ Environment Configuration

| Variable | Default (Local) | Description |
| :--- | :--- | :--- |
| `PORT` | `8080` | Port for GrutMail Web Server |
| `DATABASE_URL` | `postgres://postgres:password123@localhost:5432/email_db?sslmode=disable` | PostgreSQL connection string |
| `REDIS_URL` | *(empty)* | Optional Redis URL (e.g. `redis://user:pass@host:port`) |
| `REDIS_ADDR` | `localhost:6379` | Fallback Redis host & port |
| `SMTP_HOST` | `localhost` | SMTP Server Host |
| `SMTP_PORT` | `1025` | SMTP Server Port |
| `SENDER_EMAIL` | `gtu.tanmay@gmail.com` | From email address |
| `SMTP_USER` | *(empty)* | Optional SMTP username for production providers |
| `SMTP_PASS` | *(empty)* | Optional SMTP password / API key |

---

## ☁️ Deployment Guide (Railway / Render / VPS)

### Option 1: Railway (Recommended)
1. Push this repository to your GitHub account.
2. Sign in to [Railway.app](https://railway.app/) and click **"New Project"** -> **"Deploy from GitHub repo"**.
3. In your Railway project, click **"+ New"** -> **"Database"** -> **"PostgreSQL"**.
4. Click **"+ New"** -> **"Database"** -> **"Redis"**.
5. In your web service settings, add Environment Variables:
   - `DATABASE_URL`: `${{Postgres.DATABASE_URL}}`
   - `REDIS_URL`: `${{Redis.REDIS_URL}}`
   - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`: Your production SMTP provider (SendGrid, Brevo, AWS SES, or Gmail).
6. Railway will automatically build the `Dockerfile` and assign an HTTPS public domain!

---

## 📄 License
This project is open-source and available under the [MIT License](LICENSE).
