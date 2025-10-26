package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Digits string `json:"digits"`
	Time   string `json:"time"`
}

var (
	clients   = make(map[*websocket.Conn]struct{})
	clientsMu sync.Mutex

	dataFile  = "data.json"
	dataMu    sync.RWMutex
	dataStore []Message
)

var allowedOrigins = map[string]struct{}{
	"https://react-frontend-dq0w.onrender.com": {},
	"http://localhost:5173":                    {},
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return isAllowedOrigin(r.Header.Get("Origin"))
	},
	EnableCompression: true,
}

func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	_, ok := allowedOrigins[origin]
	return ok
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	http.ServeFile(w, r, dataFile)
}

func enableCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if isAllowedOrigin(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Add("Vary", "Origin")
}

func loadData() {
	raw, err := os.ReadFile(dataFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("data file %s not found; starting with empty history", dataFile)
			return
		}
		log.Printf("unable to read %s: %v", dataFile, err)
		return
	}

	var messages []Message
	if err := json.Unmarshal(raw, &messages); err != nil {
		log.Printf("failed to decode %s: %v", dataFile, err)
		return
	}

	dataMu.Lock()
	dataStore = messages
	dataMu.Unlock()

	log.Printf("loaded %d messages from %s", len(messages), dataFile)
}

func saveData() error {
	dataMu.RLock()
	snapshot := make([]Message, len(dataStore))
	copy(snapshot, dataStore)
	dataMu.RUnlock()

	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	if err := os.WriteFile(dataFile, payload, 0o644); err != nil {
		return fmt.Errorf("persist messages: %w", err)
	}
	return nil
}

func broadcastMessage(msg Message) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for conn := range clients {
		if err := conn.WriteJSON(msg); err != nil {
			log.Printf("broadcast failed, removing client: %v", err)
			conn.Close()
			delete(clients, conn)
		}
	}
}

func removeClient(conn *websocket.Conn) {
	clientsMu.Lock()
	if _, ok := clients[conn]; ok {
		delete(clients, conn)
	}
	clientsMu.Unlock()
	conn.Close()
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)

	if origin := r.Header.Get("Origin"); origin != "" && !isAllowedOrigin(origin) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	clientsMu.Lock()
	clients[conn] = struct{}{}
	clientsMu.Unlock()
	defer removeClient(conn)

	log.Printf("websocket client connected: %s", r.RemoteAddr)

	dataMu.RLock()
	history := make([]Message, len(dataStore))
	copy(history, dataStore)
	dataMu.RUnlock()

	for _, msg := range history {
		if err := conn.WriteJSON(msg); err != nil {
			log.Printf("failed to deliver history to client: %v", err)
			return
		}
	}

	for {
		if _, _, err := conn.NextReader(); err != nil {
			log.Printf("websocket client disconnected: %v", err)
			return
		}
	}
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)

	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodPost:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Digits string `json:"digits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Digits = strings.TrimSpace(req.Digits)
	if req.Digits == "" {
		http.Error(w, "`digits` is required", http.StatusBadRequest)
		return
	}

	msg := Message{
		Digits: req.Digits,
		Time:   time.Now().Format("15:04"),
	}

	dataMu.Lock()
	dataStore = append(dataStore, msg)
	dataMu.Unlock()

	if err := saveData(); err != nil {
		log.Printf("failed to persist message: %v", err)
	}

	broadcastMessage(msg)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(msg); err != nil {
		log.Printf("failed to write response: %v", err)
	}

	log.Printf("stored and broadcast digits: %s", req.Digits)
}

func main() {
	loadData()

	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/api/send", sendHandler)
	http.HandleFunc("/api/data", dataHandler)

	log.Println("server running on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
