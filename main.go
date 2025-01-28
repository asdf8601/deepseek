package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func listChats() {
    mutex.Lock()
    defer mutex.Unlock()

    for chatID, messages := range chatHistory {
        var lastUserMessage string
        for i := len(messages) - 1; i >= 0; i-- {
            if messages[i].Role == "user" {
                lastUserMessage = messages[i].Content
                break
            }
        }
        if lastUserMessage != "" {
            fmt.Printf("Chat ID: %s, Last user message: %s\n", chatID, lastUserMessage)
        } else {
            fmt.Printf("Chat ID: %s, No user messages.\n", chatID)
        }
    }
}

var (
	chatHistory = make(map[string][]Message)
	mutex       = &sync.Mutex{}
	historyFile string
	lastChatID  string
)

type Config struct {
	LastChatID string               `json:"last_chat_id"`
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
	fmt.Printf("Reading history from file: %s\n", historyFile) // Nueva línea añadida
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

type StreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// Generate a unique chat-id
func generateChatID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func main() {
	// Define flags
	model := flag.String("model", "deepseek-chat", "Model to use (default: deepseek-chat)")
	chatID := flag.String("chat", "", "Conversation ID (optional, generates one if not provided)")
	newChat := flag.Bool("new", false, "Create a new conversation")
	verbose := flag.Bool("v", false, "Verbose output")
	listChatsFlag := flag.Bool("ls", false, "List all chats and their last message")
	flag.Parse()

	// Check if the -ls flag was passed
	if *listChatsFlag {
	    listChats()
	    return
	}

	// Read API token from environment variable
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: DEEPSEEK_API_KEY environment variable is not set.")
		return
	}

	// Handle chat ID selection
	if *newChat || (*chatID == "" && lastChatID == "") {
		*chatID = generateChatID()
		if *verbose {
			fmt.Println("Nuevo chat-id generado:", *chatID)
		}
	} else if *chatID == "" {
		*chatID = lastChatID
		if *verbose {
			fmt.Println("Usando el último chat-id:", *chatID)
		}
	}
	lastChatID = *chatID

	// Get user prompt
	if len(flag.Args()) == 0 {
		fmt.Println("Error: You must provide a prompt as an argument.")
		return
	}
	prompt := flag.Args()[0]

	// Get message history for this chat-id
	mutex.Lock()
	messages, exists := chatHistory[*chatID]
	if !exists {
		messages = []Message{
			{Role: "system", Content: "You are a helpful assistant"},
		}
	}
	messages = append(messages, Message{Role: "user", Content: prompt})
	mutex.Unlock()

	// Build request body
	requestBody := RequestBody{
		Model:    *model,
		Messages: messages,
		Stream:   true,
	}

	// Convert body to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Println("Error marshaling request body:", err)
		return
	}

	// Create HTTP request
	url := "https://api.deepseek.com/v1/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer resp.Body.Close()

	// Process streaming response
	// fmt.Print("Response: ")
	scanner := bufio.NewScanner(resp.Body)
	var fullResponse strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		line = strings.TrimPrefix(line, "data: ")
		if line == "[DONE]" {
			break
		}

		var streamResp StreamResponse
		if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
			continue
		}

		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			fmt.Print(content)
			fullResponse.WriteString(content)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("\nError reading stream:", err)
		return
	}
	fmt.Println()

	// Update message history
	assistantMessage := fullResponse.String()
	mutex.Lock()
	chatHistory[*chatID] = append(messages, Message{Role: "assistant", Content: assistantMessage})
	mutex.Unlock()
	saveHistory()
}
