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
	"sort"
	"strings"
	"sync"
	"time"
)

func listChats() {
	mutex.Lock()
	defer mutex.Unlock()

	fmt.Printf("%-2s %-20s %-15s %-30s %-30s\n", "", "CHAT ID", "AGE", "CREATED AT", "LAST USER MESSAGE")
	fmt.Println(strings.Repeat("-", 97))

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
		if lastUserMessage != "" {
			fmt.Printf("%-2s %-20s %-15s %-30s %-30.30s\n", asterisk, entry.id, age, entry.chat.CreatedAt.Format(time.RFC3339), lastUserMessage)
		} else {
			fmt.Printf("%-2s %-20s %-15s %-30s %-30s\n", asterisk, entry.id, age, entry.chat.CreatedAt.Format(time.RFC3339), "No user messages")
		}
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

func init() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		return
	}
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
	verbose := flag.Bool("v", false, "Verbose output")
	listChatsFlag := flag.Bool("ls", false, "List all chats and their last message")
	removeChat := flag.String("rm", "", "Remove chats older than the specified duration (e.g., 10d) or by ID")
	flag.Parse()

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
		chat = Chat{
			CreatedAt: time.Now(),
			Messages: []Message{
				{Role: "system", Content: "You are a helpful assistant"},
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
	chat.Messages = append(chat.Messages, Message{Role: "assistant", Content: assistantMessage})
	chatHistory[*chatID] = chat
	mutex.Unlock()
	saveHistory()
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
