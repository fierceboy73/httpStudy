package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Digits string `json:"digits"`
	Time   string `json:"time"`
}

var (
	clients   = make(map[*websocket.Conn]bool)
	mu        sync.Mutex
	dataFile  = "data.json"
	dataStore []Message
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "https://react-frontend-dq0w.onrender.com" || origin == "http://localhost:5173"
	},
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "https://react-frontend-dq0w.onrender.com")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// --- Работа с файлом --------------------------------------------------------

func loadData() {
	file, err := os.ReadFile(dataFile)
	if err == nil {
		json.Unmarshal(file, &dataStore)
		log.Printf("📂 Загружено %d записей из %s\n", len(dataStore), dataFile)
	} else {
		log.Println("⚠️ Файл данных не найден, создаём новый при первой записи.")
	}
}

func saveData() {
	file, _ := json.MarshalIndent(dataStore, "", "  ")
	os.WriteFile(dataFile, file, 0644)
}

// --- WebSocket --------------------------------------------------------------

func wsHandler(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Ошибка при апгрейде:", err)
		return
	}
	defer conn.Close()

	mu.Lock()
	clients[conn] = true
	mu.Unlock()

	log.Println("🟢 WebSocket клиент подключён")

	// При подключении отправляем уже сохранённые данные
	mu.Lock()
	for _, msg := range dataStore {
		conn.WriteJSON(msg)
	}
	mu.Unlock()

	for {
		if _, _, err := conn.NextReader(); err != nil {
			log.Println("🔴 Клиент отключён")
			mu.Lock()
			delete(clients, conn)
			mu.Unlock()
			conn.Close()
			break
		}
	}
}

// --- POST /api/send --------------------------------------------------------

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

	msg := Message{
		Digits: req.Digits,
		Time:   time.Now().Format("15:04"),
	}

	// Добавляем в память и сохраняем в файл
	mu.Lock()
	dataStore = append(dataStore, msg)
	saveData()
	mu.Unlock()

	// Рассылаем всем клиентам
	mu.Lock()
	for conn := range clients {
		if err := conn.WriteJSON(msg); err != nil {
			log.Println("Ошибка отправки:", err)
			conn.Close()
			delete(clients, conn)
		}
	}
	mu.Unlock()

	log.Println("📨 Получено и сохранено:", req.Digits)
	w.WriteHeader(http.StatusOK)
}

func main() {
	loadData()

	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/api/send", sendHandler)

	log.Println("🚀 Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
