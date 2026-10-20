package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

// Server holds shared dependencies for HTTP API routes.
type Server struct {
	db       *sql.DB
	rdb      *redis.Client
	tmplMu   sync.RWMutex
	tmpl     *template.Template
	isBusy   bool
	isBusyMu sync.Mutex
}

// StartWebServer starts the GrutMail HTTP server on port 8080.
func StartWebServer(db *sql.DB, rdb *redis.Client, tmpl *template.Template) {
	s := &Server{
		db:   db,
		rdb:  rdb,
		tmpl: tmpl,
	}

	mux := http.NewServeMux()

	// Static web assets
	fileServer := http.FileServer(http.Dir("./web"))
	mux.Handle("/", fileServer)

	// API Routes
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/contacts", s.handleContacts)
	mux.HandleFunc("/api/contacts/reset", s.handleResetContacts)
	mux.HandleFunc("/api/template", s.handleTemplate)
	mux.HandleFunc("/api/campaign/start", s.handleStartCampaign)
	mux.HandleFunc("/api/dlq", s.handleDLQ)
	mux.HandleFunc("/api/dlq/clear", s.handleClearDLQ)
	mux.HandleFunc("/api/dlq/retry", s.handleRetryDLQ)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	fmt.Printf("\n🌐 GrutMail Web Dashboard is live at: http://localhost%s\n", port)
	fmt.Println("👉 Open your browser to customize email templates and trigger dispatches.")
	fmt.Println("==================================================")

	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("❌ HTTP Server stopped: %v", err)
	}
}

// Stats response
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var total, sent, failed, pending int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM contacts;").Scan(&total)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM contacts WHERE status = 'SENT';").Scan(&sent)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM contacts WHERE status = 'FAILED';").Scan(&failed)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM contacts WHERE status = 'PENDING';").Scan(&pending)

	dlqCount, _ := getDLQCount(r.Context(), s.rdb)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"total":     total,
		"sent":      sent,
		"failed":    failed,
		"pending":   pending,
		"dlq_count": dlqCount,
	})
}

// Contacts CRUD
func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		contacts, err := fetchAllContacts(s.db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(contacts)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
			http.Error(w, "Invalid input: Name and Email required", http.StatusBadRequest)
			return
		}

		if err := insertContact(s.db, req.Name, req.Email); err != nil {
			http.Error(w, fmt.Sprintf("Failed to add contact (might already exist): %v", err), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"success"}`))
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// Reset contacts to PENDING
func (s *Server) handleResetContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := resetContactsStatus(s.db); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success"}`))
}

// Email Template Get & Customize
func (s *Server) handleTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		content, err := os.ReadFile("email.tmpl")
		if err != nil {
			http.Error(w, "Template file not found", http.StatusInternalServerError)
			return
		}

		// Parse Subject and Body from RFC 822 template format
		text := string(content)
		lines := strings.Split(text, "\n")
		subject := ""
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

		body := ""
		if bodyStartIndex != -1 && bodyStartIndex < len(lines) {
			body = strings.Join(lines[bodyStartIndex:], "\n")
		} else {
			body = text
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"subject": subject,
			"body":    strings.TrimSpace(body),
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			return
		}

		// Rebuild standardized RFC 822 email template
		newContent := fmt.Sprintf("To: {{.Email}}\nSubject: %s\n\n%s\n", req.Subject, req.Body)

		if err := os.WriteFile("email.tmpl", []byte(newContent), 0644); err != nil {
			http.Error(w, "Failed to save email.tmpl file", http.StatusInternalServerError)
			return
		}

		// Hot-reload parsed template in memory
		newTmpl, err := template.ParseFiles("email.tmpl")
		if err != nil {
			http.Error(w, fmt.Sprintf("Template syntax error: %v", err), http.StatusBadRequest)
			return
		}

		s.tmplMu.Lock()
		s.tmpl = newTmpl
		s.tmplMu.Unlock()

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// Start Campaign Trigger
func (s *Server) handleStartCampaign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.isBusyMu.Lock()
	if s.isBusy {
		s.isBusyMu.Unlock()
		http.Error(w, "Campaign is already actively running", http.StatusConflict)
		return
	}
	s.isBusy = true
	s.isBusyMu.Unlock()

	// Launch Campaign in background
	go func() {
		defer func() {
			s.isBusyMu.Lock()
			s.isBusy = false
			s.isBusyMu.Unlock()
		}()

		recipientChannel := make(chan Recipient, 50)

		// 1. Stream pending contacts from DB
		go func() {
			if err := loadRecipientFromDB(s.db, recipientChannel); err != nil {
				log.Printf("[Web Server] Producer error: %v\n", err)
			}
		}()

		// 2. Launch worker pool
		var wg sync.WaitGroup
		workerCount := 5

		s.tmplMu.RLock()
		currentTmpl := s.tmpl
		s.tmplMu.RUnlock()

		for i := 1; i <= workerCount; i++ {
			wg.Add(1)
			go emailWorker(i, recipientChannel, currentTmpl, s.db, s.rdb, &wg)
		}

		wg.Wait()
		log.Println("[Web Server] Campaign completed successfully.")
	}()

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"campaign_started"}`))
}

// DLQ Routes
func (s *Server) handleDLQ(w http.ResponseWriter, r *http.Request) {
	messages, err := fetchDLQMessages(r.Context(), s.rdb)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(messages)
}

func (s *Server) handleClearDLQ(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := clearDLQ(r.Context(), s.rdb); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success"}`))
}

func (s *Server) handleRetryDLQ(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	messages, err := fetchDLQMessages(r.Context(), s.rdb)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, msg := range messages {
		_ = updateContactStatus(s.db, msg.Recipient.Email, "PENDING", "")
	}

	_ = clearDLQ(r.Context(), s.rdb)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"count": len(messages)})
}
