package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Submission struct {
	Digits string `json:"digits"`
	Time   string `json:"time"`
}

var (
	submissions []Submission
	clients     = make(map[*websocket.Conn]bool)
	mu          sync.Mutex
	upgrader    = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true }, // разрешаем все источники
	}
)

// --- HTTP routes ---
func main() {

	// Отдаём React-файлы
	fs := http.FileServer(http.Dir("./frontend/dist"))
	http.Handle("/", fs)

	// API и WebSocket
	http.HandleFunc("/api/send", handleSend)
	http.HandleFunc("/ws", handleWS)

	fmt.Println("🚀 Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleSend(w http.ResponseWriter, r *http.Request) {
	enableCORS(&w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Digits string `json:"digits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	mu.Lock()
	sub := Submission{Digits: body.Digits, Time: time.Now().Format("15:04")}
	submissions = append(submissions, sub)
	mu.Unlock()

	broadcast(sub) // отправляем всем клиентам обновление

	w.WriteHeader(http.StatusOK)
}

func handleList(w http.ResponseWriter, r *http.Request) {
	enableCORS(&w, r)

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	mu.Lock()
	defer mu.Unlock()
	json.NewEncoder(w).Encode(submissions)
}

// --- WebSocket part ---
func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	mu.Lock()
	clients[conn] = true
	mu.Unlock()

	log.Println("🟢 New WebSocket client connected")

	// слушаем клиент (чтобы соединение не закрывалось)
	go func(c *websocket.Conn) {
		defer func() {
			mu.Lock()
			delete(clients, c)
			mu.Unlock()
			c.Close()
			log.Println("🔴 WebSocket client disconnected")
		}()

		for {
			// читаем пустые сообщения (чтобы не отваливалось)
			if _, _, err := c.ReadMessage(); err != nil {
				break
			}
		}
	}(conn)
}

func broadcast(sub Submission) {
	mu.Lock()
	defer mu.Unlock()

	data, _ := json.Marshal(sub)
	for c := range clients {
		err := c.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			c.Close()
			delete(clients, c)
		}
	}
}

func enableCORS(w *http.ResponseWriter, r *http.Request) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type")
}
