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
- Cross-platform binaries available for Linux, macOS and Windows

## Installation

Download the latest binary for your platform from the [releases page](https://github.com/asdf8601/deepseek/releases).

### Quick install

```bash
# Linux AMD64
curl -LO $(curl -s https://api.github.com/repos/asdf8601/deepseek/releases/latest | grep browser_download_url | grep linux_amd64 | cut -d '"' -f 4)
sudo mv deepseek-linux-amd64 /usr/local/bin/deepseek

# macOS AMD64
curl -LO $(curl -s https://api.github.com/repos/asdf8601/deepseek/releases/latest | grep browser_download_url | grep darwin_amd64 | cut -d '"' -f 4)
chmod +x deepseek-darwin-amd64
sudo mv deepseek-darwin-amd64 /usr/local/bin/deepseek

# macOS ARM64 (Apple Silicon)
curl -LO $(curl -s https://api.github.com/repos/asdf8601/deepseek/releases/latest | grep browser_download_url | grep darwin_arm64 | cut -d '"' -f 4)
chmod +x deepseek-darwin-arm64
sudo mv deepseek-darwin-arm64 /usr/local/bin/deepseek
```
