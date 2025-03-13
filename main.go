package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
)

// Структура сообщения
type Message struct {
	Type    string   `json:"type"`
	Message string   `json:"message"`
	Room    string   `json:"room,omitempty"`     // Новый параметр Room, он может быть пустым
	UserIDs []string `json:"user_ids,omitempty"` // Этот параметр может быть пустым, если Room не пустой
}

// Структура для хранения WebSocket соединений с привязкой к пользователям
type Client struct {
	conn *websocket.Conn
	id   string
}

type Config struct {
	ServerMode   string `json:"server_mode"`    // http или https
	CertFilePath string `json:"cert_file_path"` // Путь к сертификату
	KeyFilePath  string `json:"key_file_path"`  // Путь к ключу
	Port         string `json:"port"`           // Порт для подключения
}

var rooms = make(map[string][]string)

var (
	clients      = make(map[string]*Client) // Хранение пользователей по ID
	clientsMutex sync.Mutex                 // Мьютекс для синхронизации доступа к клиентам
	upgrader     = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

// Загрузка конфигурации из файла
func loadConfig() (*Config, error) {
	file, err := os.Open("config.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := &Config{}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}

// Обработка сообщений от клиента через WebSocket
func handleMessages(conn *websocket.Conn, userID string) {
	defer conn.Close()

	for {
		// Чтение сообщения от клиента
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			clientsMutex.Lock()
			delete(clients, userID)
			clientsMutex.Unlock()
			break
		}

		// Парсинг JSON сообщения
		var msgObj Message
		if err := json.Unmarshal(msg, &msgObj); err != nil {
			log.Printf("Error unmarshalling message: %v", err)
			continue
		}

		// Проверка на заполненность Room и UserIDs
		if msgObj.Room != "" {
			// Отправляем сообщение всем пользователям в этой комнате
			sendToRoom(msgObj)
		} else if len(msgObj.UserIDs) > 0 {
			// Если Room пустой, отправляем по UserIDs
			clientsMutex.Lock()
			for _, id := range msgObj.UserIDs {
				if client, ok := clients[id]; ok {
					err := client.conn.WriteMessage(websocket.TextMessage, msg)
					if err != nil {
						log.Printf("Error sending message to client %s: %v", id, err)
					}
				}
			}
			clientsMutex.Unlock()
		}
	}
}

func sendToRoom(msgObj Message) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	// Получаем список пользователей в комнате
	roomID := msgObj.Room
	usersInRoom, exists := rooms[roomID]
	if !exists {
		log.Printf("Room %s does not exist", roomID)
		return
	}

	// Отправляем сообщение всем пользователям в этой комнате
	for _, userID := range usersInRoom {
		if client, ok := clients[userID]; ok {
			err := client.conn.WriteMessage(websocket.TextMessage, []byte(msgObj.Message))
			if err != nil {
				log.Printf("Error sending message to client %s: %v", userID, err)
			}
		}
	}
}

func joinRoom(userID, roomID string) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	// Добавляем пользователя в комнату
	rooms[roomID] = append(rooms[roomID], userID)
	log.Printf("User %s joined room %s", userID, roomID)
}

// Обработчик WebSocket-соединений
func handleWebSocket(w http.ResponseWriter, r *http.Request) {

	enableCors(&w)
	// Преобразование HTTP-соединения в WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading connection: %v", err)
		return
	}

	// Получаем user_id из запроса
	userID := r.URL.Query().Get("user_id")
	roomID := r.URL.Query().Get("room_id")

	if userID == "" {
		log.Println("User ID is required")
		conn.Close()
		return
	}

	// Добавляем клиента
	clientsMutex.Lock()
	clients[userID] = &Client{conn: conn, id: userID}
	clientsMutex.Unlock()

	log.Printf("User %s connected", userID)

	if roomID != "" {
		joinRoom(userID, roomID)
	}

	// Обрабатываем сообщения от клиента
	handleMessages(conn, userID)
}

// Обработчик POST запросов для отправки сообщений
func sendMessage(w http.ResponseWriter, r *http.Request) {
	// Чтение и парсинг POST запроса
	var msg Message
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Если user_ids не указаны, отправлять не будем
	if len(msg.UserIDs) == 0 {
		http.Error(w, "No user_ids provided", http.StatusBadRequest)
		return
	}

	// Отправляем сообщение указанным пользователям
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	for _, id := range msg.UserIDs {
		if client, ok := clients[id]; ok {
			// Отправляем сообщение
			messageJSON, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Error marshalling message: %v", err)
				continue
			}
			err = client.conn.WriteMessage(websocket.TextMessage, messageJSON)
			if err != nil {
				log.Printf("Error sending message to client %s: %v", id, err)
			}
		} else {
			log.Printf("Client %s not connected", id)
		}
	}

	// Ответ на POST запрос
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Message sent to %d clients", len(msg.UserIDs))
}

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Обработчики
	http.HandleFunc("/ws", handleWebSocket) // WebSocket соединения
	http.HandleFunc("/send", sendMessage)   // POST запрос для отправки сообщений

	// Запуск сервера
	port := config.Port
	if config.ServerMode == "https" {
		certFile := config.CertFilePath
		keyFile := config.KeyFilePath
		if _, err := ioutil.ReadFile(certFile); err != nil {
			log.Fatalf("Error reading cert file: %v", err)
		}
		if _, err := ioutil.ReadFile(keyFile); err != nil {
			log.Fatalf("Error reading key file: %v", err)
		}

		log.Printf("Starting HTTPS server on :%s", port)
		if err := http.ListenAndServeTLS(":"+port, certFile, keyFile, nil); err != nil {
			log.Fatalf("Error starting HTTPS server: %v", err)
		}
	} else {
		log.Printf("Starting HTTP server on :%s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("Error starting HTTP server: %v", err)
		}
	}
}
