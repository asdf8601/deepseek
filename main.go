package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Define una estructura para las columnas con toda la información necesaria
type column struct {
	id       string
	name     string
	format   string
	width    int
	getValue func(asterisk string, chatId string, age string, created string, lastMsg string) string
}

func checkServiceStatus() {
	url := "https://status.deepseek.com/api/v2/status.json"
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error fetching service status:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Failed to get service status: %s\n", resp.Status)
		return
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Println("Error parsing JSON response:", err)
		return
	}

	status := result["status"].(map[string]interface{})
	fmt.Printf("Service Status: %s - %s\n", status["indicator"], status["description"])
}

func listChats() {
	mutex.Lock()
	defer mutex.Unlock()

	// Define columns and their order
	columns := []column{
		{
			id:       "asterisk",
			name:     "",
			format:   "%-2s",
			width:    2,
			getValue: func(asterisk, _, _, _, _ string) string { return asterisk },
		},
		{
			id:       "chat_id",
			name:     "CHAT ID",
			format:   "%-18s",
			width:    18,
			getValue: func(_, chatId, _, _, _ string) string { return chatId },
		},
		{
			id:       "age",
			name:     "AGE",
			format:   "%-10s",
			width:    10,
			getValue: func(_, _, age, _, _ string) string { return age },
		},
		{
			id:       "created_at",
			name:     "CREATED AT",
			format:   "%-20s",
			width:    20,
			getValue: func(_, _, _, created, _ string) string { return created },
		},
		{
			id:       "last_message",
			name:     "LAST USER MESSAGE",
			format:   "%-30s",
			width:    30,
			getValue: func(_, _, _, _, lastMsg string) string { return lastMsg },
		},
	}

	// Build format string and print headers
	headers := make([]string, len(columns))
	values := make([]interface{}, len(columns))
	valuesFmt := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = fmt.Sprintf(col.format, col.name)
		valuesFmt[i] += col.format
	}
	fmt.Println(strings.Join(headers, " "))

	// Convert map to slice for sorting
	type chatEntry struct {
		id   string
		chat Chat
	}
	var chats []chatEntry
	for id, chat := range chatHistory {
		chats = append(chats, chatEntry{id, chat})
	}

	// Sort by creation time, newest first
	sort.Slice(chats, func(i, j int) bool {
		return chats[i].chat.CreatedAt.After(chats[j].chat.CreatedAt)
	})

	// Print each chat entry
	for _, entry := range chats {
		var lastUserMessage string
		for i := len(entry.chat.Messages) - 1; i >= 0; i-- {
			if entry.chat.Messages[i].Role == "user" {
				lastUserMessage = entry.chat.Messages[i].Content
				break
			}
		}

		asterisk := ""
		if entry.id == lastChatID {
			asterisk = "*"
		}

		age := time.Since(entry.chat.CreatedAt).Round(time.Second)
		created := entry.chat.CreatedAt.Format(time.DateTime)

		// Get values for each column
		for i, col := range columns {
			values[i] = col.getValue(asterisk, entry.id, fmt.Sprint(age), created, lastUserMessage)
		}

		// Print the row
		fmt.Printf(strings.Join(valuesFmt, " ")+"\n", values...)
	}

}

var (
	chatHistory = make(map[string]Chat)
	mutex       = &sync.Mutex{}
	historyFile string
	lastChatID  string
)

type Chat struct {
	CreatedAt time.Time `json:"created_at"`
	Messages  []Message `json:"messages"`
}

type Config struct {
	LastChatID string          `json:"last_chat_id"`
	History    map[string]Chat `json:"history"`
}

var checkStatus *bool

func init() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		return
	}

	checkStatus = flag.Bool("status", false, "Check DeepSeek service status")
	historyFile = filepath.Join(homeDir, ".deepseek_history.json")
	loadHistory(historyFile)
}

func loadHistory(historyFile string) {
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
	chatHistory = make(map[string]Chat)
	if config.History != nil {
		chatHistory = config.History
	} else {
		chatHistory = make(map[string]Chat)
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
	debug := flag.Bool("debug", false, "Enable debug logging")
	listChatsFlag := flag.Bool("ls", false, "List all chats and their last message")
	checkModels := flag.Bool("models", false, "List available Deepseek models")
	removeChat := flag.String("rm", "", "Remove chats older than the specified duration (e.g., 10d) or by ID")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	flag.Parse()
	// Check if the -status flag was passed
	if *checkStatus {
		checkServiceStatus()
		return
	}
	if *checkModels {
		listDeepseekModels()
		return
	}

	// Check if the -rm flag was passed
	if *removeChat != "" {
		removeChats(*removeChat)
		saveHistory()
		return
	}

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
			fmt.Println("New chat-id generated:", *chatID)
		}
	} else if *chatID == "" {
		*chatID = lastChatID
		if *verbose {
			fmt.Println("Using last chat-id:", *chatID)
		}
	}
	lastChatID = *chatID

	// Get user prompt
	if len(flag.Args()) == 0 {
		fmt.Println("Error: You must provide a prompt as an argument.")
		return
	}
	prompt := flag.Args()[0]

	// Get chat history for this chat-id
	mutex.Lock()
	chat, exists := chatHistory[*chatID]
	if !exists {
		sys_content := os.Getenv("DEEPSEEK_ROLE")
		if sys_content == "" {
			sys_content = "You are a helpful assistant. Be concise."
		}

		chat = Chat{
			CreatedAt: time.Now(),
			Messages: []Message{
				{Role: "system", Content: sys_content},
			},
		}
	}
	chat.Messages = append(chat.Messages, Message{Role: "user", Content: prompt})
	chatHistory[*chatID] = chat
	mutex.Unlock()

	// Build request body
	requestBody := RequestBody{
		Model:    *model,
		Messages: chat.Messages,
		Stream:   true,
	}

	// Convert body to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Println("Error marshaling request body:", err)
		return
	}

	if *debug {
		log.Printf("Request body: %s\n", string(jsonData))
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

	if *debug {
		log.Println("=== Request headers:")
		for key, values := range req.Header {
			log.Printf("  %s: %v\n", key, values)
		}
	}

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer resp.Body.Close()

	if *debug {
		log.Printf("=== Response status: %s\n", resp.Status)
		log.Println("=== Response headers:")
		for key, values := range resp.Header {
			log.Printf("  %s: %v\n", key, values)
		}
	}

	// Check if the response status is not 200
	if resp.StatusCode != http.StatusOK {
		// Read and log the error response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading error response: %v\n", err)
			return
		}
		log.Printf("API Error Response: %s\n", string(body))
		return
	}

	// Process streaming response
	scanner := bufio.NewScanner(resp.Body)
	var fullResponse strings.Builder

	if *debug {
		log.Println("=== Starting to process stream response...")
	}

	for scanner.Scan() {
		line := scanner.Text()
		if *debug {
			log.Printf("== Raw line received: %s\n", line)
		}

		if line == "" {
			if *debug {
				log.Println("Empty line, skipping")
			}
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			if *debug {
				log.Printf("Line doesn't start with 'data: ', skipping: %s\n", line)
			}
			continue
		}

		line = strings.TrimPrefix(line, "data: ")
		if line == "[DONE]" {
			if *debug {
				log.Println("Received [DONE] message, ending stream")
			}
			break
		}

		var streamResp StreamResponse
		if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
			if *debug {
				log.Printf("Error unmarshaling JSON: %v\nProblematic line: %s\n", err, line)
			}
			continue
		}

		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			if *debug {
				log.Printf("Received content chunk: %s\n", content)
			}
			fmt.Print(content)
			fullResponse.WriteString(content)
		} else if *debug {
			log.Println("No choices in response")
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
	chat.Messages = append(chat.Messages, Message{Role: "assistant", Content: assistantMessage})
	chatHistory[*chatID] = chat
	mutex.Unlock()
	saveHistory()
}

func listDeepseekModels() {
	// Read API token from environment variable
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: DEEPSEEK_API_KEY environment variable is not set.")
		return
	}

	// Create a simple request to check models endpoint
	url := "https://api.deepseek.com/v1/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Send request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	if resp.StatusCode == http.StatusOK {
		// Pretty print the available models
		var prettyJSON bytes.Buffer
		err = json.Indent(&prettyJSON, body, "", "  ")
		if err != nil {
			fmt.Printf("Error formatting JSON: %v\n", err)
			return
		}
		fmt.Println("Available model IDs:")
		var responseData map[string]interface{}
		if err := json.Unmarshal(body, &responseData); err != nil {
			fmt.Printf("Error parsing JSON: %v\n", err)
			return
		}
		if data, ok := responseData["data"].([]interface{}); ok {
			for _, item := range data {
				if model, ok := item.(map[string]interface{}); ok {
					fmt.Println(model["id"])
				}
			}
		}
	} else {
		fmt.Printf("Response: %s\n", string(body))
	}
}

func removeChats(criteria string) {
	mutex.Lock()
	defer mutex.Unlock()

	// Try to parse as duration
	duration, err := time.ParseDuration(criteria)
	if err == nil {
		cutoff := time.Now().Add(-duration)
		fmt.Printf("Removing chats older than: %s\n", cutoff)

		// Remove chats older than the cutoff
		removed := false
		for chatID, chat := range chatHistory {
			if chat.CreatedAt.Before(cutoff) {
				delete(chatHistory, chatID)
				fmt.Printf("Chat ID: %s removed due to age.\n", chatID)
				removed = true
			}
		}
		if !removed {
			fmt.Println("No chats were removed. All chats are within the specified duration.")
		}
		return
	}

	// Try to remove by ID
	if _, exists := chatHistory[criteria]; exists {
		delete(chatHistory, criteria)
		fmt.Printf("Chat ID: %s removed.\n", criteria)
	} else {
		fmt.Println("Invalid input: not a valid duration or chat ID.")
	}
}
