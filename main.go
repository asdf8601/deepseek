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
	"sync"
)

var (
	chatHistory = make(map[string][]Message)
	mutex       = &sync.Mutex{}
)

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

	// Generar un chat-id si no se proporciona
	if *chatID == "" {
		*chatID = generateChatID()
		fmt.Println("Nuevo chat-id generado:", *chatID)
	}

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
	} else {
		fmt.Println("No se recibió ninguna respuesta del modelo.")
	}
}
