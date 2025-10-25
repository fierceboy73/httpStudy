package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Time  string `json:"time"`
	Value string `json:"value"`
}

var (
	clients   = make(map[*websocket.Conn]bool)
	broadcast = make(chan Message)
	upgrader  = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	messages []Message
	mutex    sync.Mutex
)

// ===== Работа с файлом =====

func loadMessages() {
	file, err := os.Open("data.json")
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("💾 data.json не найден — создаём новый.")
			messages = []Message{}
			return
		}
		log.Println("Ошибка чтения data.json:", err)
		return
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&messages)
	if err != nil {
		log.Println("Ошибка парсинга JSON:", err)
	}
	log.Printf("📂 Загружено %d сообщений из data.json\n", len(messages))
}

func saveMessages() {
	file, err := os.Create("data.json")
	if err != nil {
		log.Println("Ошибка записи data.json:", err)
		return
	}
	defer file.Close()

	err = json.NewEncoder(file).Encode(messages)
	if err != nil {
		log.Println("Ошибка кодирования JSON:", err)
	}
}

// ===== Основные обработчики =====

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Ошибка WebSocket:", err)
		return
	}
	defer conn.Close()

	mutex.Lock()
	clients[conn] = true

	// При подключении отправляем все старые сообщения
	conn.WriteJSON(messages)
	mutex.Unlock()

	log.Println("🟢 Новый WebSocket клиент подключен")

	for {
		if _, _, err := conn.NextReader(); err != nil {
			mutex.Lock()
			delete(clients, conn)
			mutex.Unlock()
			log.Println("🔴 Клиент отключился")
			break
		}
	}
}

func handleSend(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	msg := Message{
		Time:  time.Now().Format("15:04"),
		Value: data.Value,
	}

	mutex.Lock()
	messages = append(messages, msg)
	saveMessages() // сохраняем в файл
	mutex.Unlock()

	broadcast <- msg
	w.WriteHeader(http.StatusOK)
}

// ===== Поток для рассылки =====

func handleMessages() {
	for {
		msg := <-broadcast
		mutex.Lock()
		for client := range clients {
			err := client.WriteJSON([]Message{msg})
			if err != nil {
				log.Println("Ошибка отправки:", err)
				client.Close()
				delete(clients, client)
			}
		}
		mutex.Unlock()
	}
}

// ===== main =====

func main() {
	loadMessages() // загружаем историю при старте

	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/api/send", handleSend)

	go handleMessages()

	fmt.Println("🚀 Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
