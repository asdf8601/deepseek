package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var (
	chatHistory = make(map[string][]Message)
	mutex       = &sync.Mutex{}
	historyFile string
	lastChatID  string
)

type Config struct {
	LastChatID string            `json:"last_chat_id"`
	History    map[string][]Message `json:"history"`
}

func init() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		return
	}
	historyFile = filepath.Join(homeDir, ".deepseek_history.json")
	loadHistory()
}

func loadHistory() {
	mutex.Lock()
	defer mutex.Unlock()

	data, err := os.ReadFile(historyFile)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Println("Error reading history file:", err)
		}
		return
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		fmt.Println("Error parsing history file:", err)
		return
	}
	if config.History != nil {
		chatHistory = config.History
	} else {
		chatHistory = make(map[string][]Message)
	}
	lastChatID = config.LastChatID
}

func saveHistory() {
	mutex.Lock()
	defer mutex.Unlock()

	config := Config{
		LastChatID: lastChatID,
		History:    chatHistory,
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fmt.Println("Error marshaling history:", err)
		return
	}

	err = os.WriteFile(historyFile, data, 0600)
	if err != nil {
		fmt.Println("Error writing history file:", err)
	}
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RequestBody struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type ResponseBody struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Genera un chat-id único
func generateChatID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func main() {
	// Definir flags
	model := flag.String("model", "deepseek-chat", "Modelo a utilizar (por defecto: deepseek-chat)")
	chatID := flag.String("chat", "", "ID de la conversación (opcional, se genera uno si no se proporciona)")
	flag.Parse()

	// Leer el token de la API de la variable de entorno
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: La variable de entorno DEEPSEEK_API_KEY no está definida.")
		return
	}

	// Usar el último chat-id o generar uno nuevo
	if *chatID == "" {
		if lastChatID != "" {
			*chatID = lastChatID
			fmt.Println("Usando el último chat-id:", *chatID)
		} else {
			*chatID = generateChatID()
			fmt.Println("Nuevo chat-id generado:", *chatID)
		}
	}
	lastChatID = *chatID

	// Obtener el prompt del usuario
	if len(flag.Args()) == 0 {
		fmt.Println("Error: Debes proporcionar un prompt como argumento.")
		return
	}
	prompt := flag.Args()[0]

	// Obtener el historial de mensajes para este chat-id
	mutex.Lock()
	messages, exists := chatHistory[*chatID]
	if !exists {
		messages = []Message{
			{Role: "system", Content: "You are a helpful assistant"},
		}
	}
	messages = append(messages, Message{Role: "user", Content: prompt})
	mutex.Unlock()

	// Construir el cuerpo de la solicitud
	requestBody := RequestBody{
		Model:    *model,
		Messages: messages,
		Stream:   false,
	}

	// Convertir el cuerpo a JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Println("Error marshaling request body:", err)
		return
	}

	// Crear la solicitud HTTP
	url := "https://api.deepseek.com/v1/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// Configurar los encabezados
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Enviar la solicitud
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer resp.Body.Close()

	// Leer la respuesta
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}

	// Decodificar la respuesta
	var response ResponseBody
	err = json.Unmarshal(body, &response)
	if err != nil {
		fmt.Println("Error unmarshaling response body:", err)
		return
	}

	// Mostrar la respuesta
	if len(response.Choices) > 0 {
		assistantMessage := response.Choices[0].Message.Content
		fmt.Println("Respuesta:", assistantMessage)

		// Actualizar el historial de mensajes
		mutex.Lock()
		chatHistory[*chatID] = append(messages, Message{Role: "assistant", Content: assistantMessage})
		mutex.Unlock()
		saveHistory()
	} else {
		fmt.Println("No se recibió ninguna respuesta del modelo.")
	}
}
