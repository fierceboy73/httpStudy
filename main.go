package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// Список всех подключенных клиентов
var clients = make(map[*websocket.Conn]bool)
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Разрешаем подключения с фронта
		return r.Header.Get("Origin") == "https://react-frontend-dq0w.onrender.com" || r.Header.Get("Origin") == "http://localhost:5173"
	},
}

// Сообщение, которое рассылается фронту
type Message struct {
	Digits string `json:"digits"`
	Time   string `json:"time"`
}

// WebSocket handler
func wsHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Ошибка при апгрейде:", err)
		return
	}
	defer conn.Close()
	clients[conn] = true
	log.Println("🟢 WebSocket клиент подключён")

	for {
		// Если клиент закрывает соединение
		if _, _, err := conn.NextReader(); err != nil {
			log.Println("🔴 WebSocket клиент отключён")
			delete(clients, conn)
			conn.Close()
			break
		}
	}
}

// CORS
func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "https://react-frontend-dq0w.onrender.com")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// POST /api/send
func sendHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req struct {
		Digits string `json:"digits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Создаём сообщение для всех клиентов
	msg := Message{
		Digits: req.Digits,
		Time:   fmt.Sprintf("%02d:%02d", r.Context().Value(http.ServerContextKey)),
	}

	// Рассылаем сообщение всем подключённым клиентам
	for conn := range clients {
		if err := conn.WriteJSON(msg); err != nil {
			log.Println("Ошибка отправки:", err)
			conn.Close()
			delete(clients, conn)
		}
	}

	w.WriteHeader(http.StatusOK)
	log.Println("📨 Получено и отправлено:", req.Digits)
}

func main() {
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/api/send", sendHandler)

	log.Println("🚀 Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
