# 🚀 GrutMail - High-Throughput Email Engine

**GrutMail** is an aesthetic, high-throughput email dispatching system built with Go, PostgreSQL, Redis DLQ, and Mailpit.

---

## 1. Quick Start

### Step 1: Start Docker Infrastructure (Mailpit + PostgreSQL + Redis)
```powershell
docker compose up -d
```

### Step 2: Run the GrutMail Application
```powershell
go run .
```

### Step 3: Open GrutMail Web Dashboard
Open your browser and navigate to:
👉 **[http://localhost:8080](http://localhost:8080)**

---

## 2. Infrastructure Services & Ports

| Service | Address | Description |
| :--- | :--- | :--- |
| **GrutMail Web UI** | `http://localhost:8080` | Clean, monochromatic dashboard to customize templates & run dispatches |
| **Mailpit Inbox** | `http://localhost:8025` | Web dashboard to view sent emails in real-time |
| **Mailpit SMTP** | `localhost:1025` | SMTP port Go code connects to |
| **PostgreSQL** | `localhost:5432` | DB: `email_db`, User: `postgres`, Password: `password123` |
| **Redis** | `localhost:6379` | Dead Letter Queue (`email:dlq`) store |


---

## 3. How to Run the Go Application

1. Open terminal inside the project directory:
   ```powershell
   cd email-dispatcher-yt
   ```

2. Download Go dependencies:
   ```powershell
   go mod tidy
   ```

3. Run the application:
   ```powershell
   go run .
   ```

---

## 4. Useful Inspection Commands

### Check PostgreSQL Contacts:
```powershell
docker exec -it email_postgres psql -U postgres -d email_db -c "SELECT id, name, email, status, error_message FROM contacts;"
```

### Inspect Redis DLQ (Dead Letter Queue):
```powershell
docker exec -it email_redis redis-cli LRANGE email:dlq 0 -1
```
