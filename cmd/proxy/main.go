package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	claudecodeproxy "claude-proxy"
)

const (
	OpenAIProxyURL = "https://cope.duti.dev"
	ListenAddr     = ":8082"
)

func main() {
	http.HandleFunc("/v1/messages", handleClaudeMessages)
	http.HandleFunc("/v1/messages/count_tokens", handleClaudeCountTokens)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message": "Claude Proxy for OpenAI"}`))
	})
	log.Printf("Claude proxy listening on %s", ListenAddr)
	log.Fatal(http.ListenAndServe(ListenAddr, nil))
}

func handleClaudeMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var claudeReq claudecodeproxy.ClaudeMessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&claudeReq); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Convert Claude request to OpenAI request
	oaiReq, err := claudecodeproxy.ConvertClaudeToOAI(claudeReq)
	if err != nil {
		http.Error(w, "Conversion error: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Marshal OAI request
	oaiBody, err := json.Marshal(oaiReq)
	if err != nil {
		http.Error(w, "Marshal error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Always stream from OpenAI, even if user requested non-stream.
	// We'll buffer and convert to non-stream if needed.
	oaiReq.Stream = true

	// Marshal OAI request (again, in case Stream changed)
	oaiBody, err = json.Marshal(oaiReq)
	if err != nil {
		http.Error(w, "Marshal error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Forward to OpenAI proxy
	req, err := http.NewRequest("POST", OpenAIProxyURL+"/chat/completions", bytes.NewReader(oaiBody))
	if err != nil {
		http.Error(w, "Request error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	// Forward API key if present
	if apiKey := os.Getenv("COPILOT_API_KEY"); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Proxy error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("WARNING: Upstream returned non-200 status: %d %s", resp.StatusCode, body)
	}

	if claudeReq.Stream != nil && *claudeReq.Stream {
		// User requested streaming, so proxy as stream
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		claudecodeproxy.ConvertOAIStreamToClaudeStream(resp.Body, w, claudeReq.Model)
		return
	} else {
		// User requested non-stream, so buffer the stream and convert to non-stream response
		var buf bytes.Buffer
		err := claudecodeproxy.ConvertOAIStreamToClaudeStream(resp.Body, &buf, claudeReq.Model)
		if err != nil {
			http.Error(w, "Stream conversion error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Now parse the buffered events to reconstruct a ClaudeMessagesResponse
		claudeResp, err := claudecodeproxy.ParseClaudeStreamToResponse(&buf)
		if err != nil {
			http.Error(w, "Claude stream parse error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Log the output of claudeResp for verification
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claudeResp)
		return
	}
}

func handleClaudeCountTokens(w http.ResponseWriter, r *http.Request) {
	respObj := map[string]int{"input_tokens": 0}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(respObj)
}

// Helper: Convert OAIResponse to ClaudeMessagesResponse
func oaiToClaudeResponse(oaiResp claudecodeproxy.OAIResponse, origReq claudecodeproxy.ClaudeMessagesRequest) (claudecodeproxy.ClaudeMessagesResponse, error) {
	// Only support single choice for now
	if len(oaiResp.Choices) == 0 {
		return claudecodeproxy.ClaudeMessagesResponse{}, nil
	}
	choice := oaiResp.Choices[0].(map[string]any)
	message := choice["message"].(map[string]any)
	content := message["content"]
	claudeContent := []any{}
	if s, ok := content.(string); ok {
		claudeContent = append(claudeContent, map[string]any{"type": "text", "text": s})
	}
	stopReason := "end_turn"
	if fr, ok := choice["finish_reason"].(string); ok {
		switch fr {
		case "length":
			stopReason = "max_tokens"
		case "tool_calls":
			stopReason = "tool_use"
		case "stop":
			stopReason = "end_turn"
		}
	}
	return claudecodeproxy.ClaudeMessagesResponse{
		ID:           oaiResp.ID,
		Model:        origReq.Model,
		Role:         "assistant",
		Content:      claudeContent,
		Type:         "message",
		StopReason:   &stopReason,
		StopSequence: nil,
		Usage: claudecodeproxy.ClaudeUsage{
			InputTokens:              oaiResp.Usage.PromptTokens,
			OutputTokens:             oaiResp.Usage.CompletionTokens,
			CacheCreationInputTokens: 0,
			CacheReadInputTokens:     0,
		},
	}, nil
}
