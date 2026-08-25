// Package ai is a tiny OpenAI-compatible chat client for the RAG answer step.
// It reads config from env (OPENAI_API_KEY, OPENAI_BASE_URL, AI_MODEL) so the
// key never lives in the repo. Retrieval is done in Postgres (FTS); this only
// synthesizes the answer from the retrieved context.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// resolve picks the provider config from env, preferring Gemini when its key is
// set. Gemini exposes an OpenAI-compatible surface, so the same Chat() works for
// both — only key/base/model differ.
func resolve() (key, base, mdl string) {
	// Accept either name (GOOGLE_API_KEY is the Google GenAI SDK convention).
	k := os.Getenv("GEMINI_API_KEY")
	if k == "" {
		k = os.Getenv("GOOGLE_API_KEY")
	}
	if k != "" {
		b := os.Getenv("GEMINI_BASE_URL")
		if b == "" {
			b = "https://generativelanguage.googleapis.com/v1beta/openai"
		}
		m := os.Getenv("AI_MODEL")
		if m == "" {
			m = "gemini-2.5-flash"
		}
		return k, strings.TrimRight(b, "/"), m
	}
	b := os.Getenv("OPENAI_BASE_URL")
	if b == "" {
		b = "https://api.openai.com/v1"
	}
	m := os.Getenv("AI_MODEL")
	if m == "" {
		m = "cx/gpt-5.4-mini"
	}
	return os.Getenv("OPENAI_API_KEY"), strings.TrimRight(b, "/"), m
}

// Configured reports whether a chat endpoint is usable.
func Configured() bool { k, _, _ := resolve(); return k != "" }

// Model returns the chat model that would be used (for display/telemetry).
func Model() string { _, _, m := resolve(); return m }

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
}

// Chat sends one system+user turn and returns the assistant text.
func Chat(ctx context.Context, system, user string) (string, error) {
	key, base, mdl := resolve()
	if key == "" {
		return "", errors.New("AI belum dikonfigurasi (set GEMINI_API_KEY atau OPENAI_API_KEY)")
	}
	body, _ := json.Marshal(chatRequest{
		Model:       mdl,
		MaxTokens:   800,
		Temperature: 0.2,
		Messages:    []message{{Role: "system", Content: system}, {Role: "user", Content: user}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	res, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var out struct {
		Choices []struct {
			Message message `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("ai: bad response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("ai: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("ai: empty response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
