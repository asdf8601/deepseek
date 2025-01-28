# DeepSeek CLI

A command-line interface for DeepSeek's AI chat models.

## Setup

1. Get your API key from [DeepSeek](https://platform.deepseek.com)
2. Set your API key:
```bash
export DEEPSEEK_API_KEY="your-api-key"
```

## Usage

Basic chat:
```bash
go run main.go "Your message here"
```

Create new conversation:
```bash
go run main.go -new "Start new chat"
```

Use specific chat ID:
```bash
go run main.go -chat abc123 "Continue specific chat"
```

## Features

- Persistent chat history
- Streaming responses
- Multiple conversation support
