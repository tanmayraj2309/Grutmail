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

		// 2. Dispatch email via SMTP server
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
