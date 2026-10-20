package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// emailWorker consumes recipients from the channel, sends emails, updates PostgreSQL status,
// and redirects failed deliveries to the Redis Dead Letter Queue (DLQ).
func emailWorker(
	id int,
	ch chan Recipient,
	tmpl *template.Template,
	db *sql.DB,
	rdb *redis.Client,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if smtpHost == "" {
		smtpHost = "localhost"
	}
	smtpPort := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if smtpPort == "" {
		smtpPort = "1025"
	}
	senderEmail := strings.TrimSpace(os.Getenv("SENDER_EMAIL"))
	if senderEmail == "" {
		senderEmail = "gtu.tanmay@gmail.com"
	}

	// Support SMTP authentication for production email providers (Gmail, Brevo, SendGrid, etc.)
	var auth smtp.Auth
	smtpUser := strings.TrimSpace(os.Getenv("SMTP_USER"))
	// Strip any spaces from Google App Password (e.g. 'zzan mhvk ...' -> 'zzanmhvk...')
	smtpPass := strings.ReplaceAll(strings.TrimSpace(os.Getenv("SMTP_PASS")), " ", "")
	if smtpUser != "" && smtpPass != "" {
		auth = smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	}

	// Check if running in Cloud Demo / Simulation mode
	isDemoMode := os.Getenv("DEMO_MODE") == "true" || smtpHost == "demo" || smtpHost == "mock" || smtpHost == "simulation"

	for recipient := range ch {
		// 1. Render dynamic email from memory-cached template
		msg, err := executeTemplate(tmpl, recipient)
		if err != nil {
			errMsg := fmt.Sprintf("Template rendering failed: %v", err)
			log.Printf("❌ [Worker %d] %s for %s\n", id, errMsg, recipient.Email)

			// Record failure in PostgreSQL & push to Redis DLQ
			_ = updateContactStatus(db, recipient.Email, "FAILED", errMsg)
			_ = pushToDLQ(context.Background(), rdb, recipient, errMsg, id)
			continue
		}

		fmt.Printf("📨 [Worker %d] Dispatching email to: %s\n", id, recipient.Email)

		// 2. Demo / Simulation Mode for Cloud Portfolios (Bypasses Render's port 587 block)
		if isDemoMode {
			time.Sleep(100 * time.Millisecond) // realistic delivery latency
			
			// If recipient has 'fail' in email OR is user25, simulate failure & push to Redis DLQ to showcase DLQ feature
			if strings.Contains(strings.ToLower(recipient.Email), "fail") || recipient.Email == "user25@example.com" {
				errMsg := "Simulated bounce: recipient mailbox does not exist (550 User Unknown)"
				log.Printf("⚠️ [Worker %d] Demo Simulated Failure for %s -> Pushed to Redis DLQ\n", id, recipient.Email)
				_ = updateContactStatus(db, recipient.Email, "FAILED", errMsg)
				_ = pushToDLQ(context.Background(), rdb, recipient, errMsg, id)
			} else {
				_ = updateContactStatus(db, recipient.Email, "SENT", "")
				fmt.Printf("✅ [Worker %d] Successfully delivered to: %s\n", id, recipient.Email)
			}
			continue
		}

		// 3. Dispatch email via Real SMTP server
		err = smtp.SendMail(
			smtpHost+":"+smtpPort,
			auth,
			senderEmail,
			[]string{recipient.Email},
			[]byte(msg),
		)

		if err != nil {
			// Production fix: DO NOT crash the program with log.Fatal!
			// Log error, mark FAILED in Postgres, and push to Redis DLQ.
			errMsg := fmt.Sprintf("SMTP send error: %v", err)
			log.Printf("⚠️ [Worker %d] Failed to send to %s: %v\n", id, recipient.Email, err)

			_ = updateContactStatus(db, recipient.Email, "FAILED", errMsg)
			_ = pushToDLQ(context.Background(), rdb, recipient, errMsg, id)
		} else {
			// 3. Mark successfully SENT in PostgreSQL
			_ = updateContactStatus(db, recipient.Email, "SENT", "")
			fmt.Printf("✅ [Worker %d] Successfully delivered to: %s\n", id, recipient.Email)
		}

		// 4. Rate-limiting throttle
		time.Sleep(50 * time.Millisecond)
	}
}
