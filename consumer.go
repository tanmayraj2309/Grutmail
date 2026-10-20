package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
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

	// Resend HTTP API Key (Works 100% on Render over HTTPS Port 443 without port 587 blocks)
	resendAPIKey := strings.TrimSpace(os.Getenv("RESEND_API_KEY"))

	// Support SMTP authentication for production email providers
	var auth smtp.Auth
	smtpUser := strings.TrimSpace(os.Getenv("SMTP_USER"))
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

		// 2. Dispatch via Resend HTTP API (Port 443 - Never blocked on Render cloud)
		if resendAPIKey != "" {
			subject, body := parseEmailMessage(msg)
			err = sendViaResend(resendAPIKey, senderEmail, recipient.Email, subject, body)
		} else if isDemoMode {
			// 3. Demo / Simulation Mode for Cloud Portfolios (Bypasses Render's port 587 block)
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
		} else {
			// 4. Dispatch email via Real SMTP server
			err = smtp.SendMail(
				smtpHost+":"+smtpPort,
				auth,
				senderEmail,
				[]string{recipient.Email},
				[]byte(msg),
			)
		}

		if err != nil {
			// Record failure in PostgreSQL & push to Redis DLQ
			errMsg := fmt.Sprintf("Delivery error: %v", err)
			log.Printf("⚠️ [Worker %d] Failed to send to %s: %v\n", id, recipient.Email, err)

			_ = updateContactStatus(db, recipient.Email, "FAILED", errMsg)
			_ = pushToDLQ(context.Background(), rdb, recipient, errMsg, id)
		} else {
			// Mark successfully SENT in PostgreSQL
			_ = updateContactStatus(db, recipient.Email, "SENT", "")
			fmt.Printf("✅ [Worker %d] Successfully delivered to: %s\n", id, recipient.Email)
		}

		// Rate-limiting throttle
		time.Sleep(50 * time.Millisecond)
	}
}

// sendViaResend sends a real email using Resend HTTP API over standard Port 443 HTTPS (Never blocked by cloud firewalls).
func sendViaResend(apiKey, fromEmail, toEmail, subject, body string) error {
	from := "GrutMail <onboarding@resend.dev>"
	if fromEmail != "" && !strings.Contains(fromEmail, "gmail.com") {
		from = fromEmail
	}

	payload := map[string]interface{}{
		"from":    from,
		"to":      []string{toEmail},
		"subject": subject,
		"text":    body,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend api error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// parseEmailMessage extracts Subject and Body from RFC 822 formatted template output.
func parseEmailMessage(raw string) (string, string) {
	lines := strings.Split(raw, "\n")
	subject := "GrutMail Campaign Update"
	bodyStartIndex := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "subject:") {
			subject = strings.TrimSpace(line[strings.Index(line, ":")+1:])
		}
		if trimmed == "" && bodyStartIndex == -1 && i > 0 {
			bodyStartIndex = i + 1
		}
	}

	body := raw
	if bodyStartIndex != -1 && bodyStartIndex < len(lines) {
		body = strings.TrimSpace(strings.Join(lines[bodyStartIndex:], "\n"))
	}

	return subject, body
}
