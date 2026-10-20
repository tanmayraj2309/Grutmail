package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"

	"github.com/redis/go-redis/v9"
)

// Recipient represents a contact to dispatch an email to.
type Recipient struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	fmt.Println("==================================================")
	fmt.Println("🚀 Starting Concurrent Email Dispatcher Engine")
	fmt.Println("==================================================")

	// 1. Pre-compile Email Template once at startup (Huge Performance Optimization)
	tmpl, err := template.ParseFiles("email.tmpl")
	if err != nil {
		log.Fatalf("❌ Failed to parse email template: %v", err)
	}

	// 2. Initialize PostgreSQL for contact persistence & status tracking
	db, err := initPostgres()
	if err != nil {
		log.Fatalf("❌ Failed to connect to PostgreSQL: %v\n(Make sure 'docker compose up -d' is running)", err)
	}
	defer db.Close()
	fmt.Println("✅ [PostgreSQL] Connected and contacts table verified.")

	// 3. Seed contacts from CSV into PostgreSQL (skips duplicates via ON CONFLICT)
	if err := seedContactsFromCSV(db, "./emails.csv"); err != nil {
		log.Printf("⚠️ Warning during CSV seeding: %v\n", err)
	}

	// 4. Initialize Redis for Dead Letter Queue (DLQ)
	rdb, err := initRedis()
	if err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v\n(Make sure 'docker compose up -d' is running)", err)
	}
	defer rdb.Close()
	fmt.Println("✅ [Redis] Connected for Dead Letter Queue (DLQ).")

	// 5. Start GrutMail Web Application Server
	StartWebServer(db, rdb, tmpl)
}

// executeTemplate renders the email body using the pre-compiled template in memory.
func executeTemplate(tmpl *template.Template, r Recipient) (string, error) {
	var tpl bytes.Buffer
	err := tmpl.Execute(&tpl, r)
	if err != nil {
		return "", err
	}
	return tpl.String(), nil
}

// printSummary outputs the final execution stats from PostgreSQL and Redis DLQ.
func printSummary(db *sql.DB, rdb *redis.Client) {
	fmt.Println("\n==================================================")
	fmt.Println("📊 Campaign Execution Summary")
	fmt.Println("==================================================")

	var sentCount, failedCount, pendingCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM contacts WHERE status = 'SENT'").Scan(&sentCount)
	_ = db.QueryRow("SELECT COUNT(*) FROM contacts WHERE status = 'FAILED'").Scan(&failedCount)
	_ = db.QueryRow("SELECT COUNT(*) FROM contacts WHERE status = 'PENDING'").Scan(&pendingCount)

	dlqCount, _ := getDLQCount(context.Background(), rdb)

	fmt.Printf("✅ Total Sent:     %d\n", sentCount)
	fmt.Printf("❌ Total Failed:   %d\n", failedCount)
	fmt.Printf("⏳ Still Pending:  %d\n", pendingCount)
	fmt.Printf("📫 Redis DLQ Size: %d messages\n", dlqCount)
	fmt.Println("==================================================")
}
