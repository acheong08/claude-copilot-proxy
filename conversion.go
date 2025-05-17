package claudecodeproxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
)

func ConvertToolChoiceClaudeToOAI(toolChoice any) any {
	if toolChoice == nil {
		return nil
	}
	tc, ok := toolChoice.(map[string]any)
	if !ok {
		return toolChoice
	}
	choiceType, _ := tc["type"].(string)
	switch choiceType {
	case "auto":
		return "auto"
	case "any":
		return "any"
	case "tool":
		if name, ok := tc["name"].(string); ok {
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": name,
				},
			}
		}
	}
	return "auto"
}

// ParseClaudeStreamToResponse parses a buffered Claude stream (as produced by ConvertOAIStreamToClaudeStream)
// and reconstructs a ClaudeMessagesResponse for non-streaming use.
func ParseClaudeStreamToResponse(r io.Reader) (ClaudeMessagesResponse, error) {
	var resp ClaudeMessagesResponse
	var contentBlocks []any
	var stopReason *string
	var stopSequence *string
	var usage ClaudeUsage
	var id string
	var model string

	var toolUseBlocks []*ClaudeContentBlockToolUse
	var currentToolUseBlock *ClaudeContentBlockToolUse
	var currentToolInputBuilder strings.Builder
	var currentTextBlock *ClaudeContentBlockText

	dec := json.NewDecoder(&eventStreamStripper{r: r})

	for {
		var event struct {
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if err := dec.Decode(&event); err != nil {
			break
		}
		switch event.Event {
		case "message_start":
			var msg struct {
				Message struct {
					ID    string      `json:"id"`
					Model string      `json:"model"`
					Usage ClaudeUsage `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal(event.Data, &msg); err == nil {
				id = msg.Message.ID
				model = msg.Message.Model
				usage = msg.Message.Usage
			}
		case "content_block_start":
			var cb struct {
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id,omitempty"`
					Name string `json:"name,omitempty"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal(event.Data, &cb); err == nil {
				switch cb.ContentBlock.Type {
				case "text":
					textBlock := &ClaudeContentBlockText{Type: "text", Text: ""}
					contentBlocks = append(contentBlocks, textBlock)
					currentTextBlock = textBlock
					currentToolUseBlock = nil
					currentToolInputBuilder.Reset()
				case "tool_use":
					if cb.ContentBlock.ID != "" && cb.ContentBlock.Name != "" {
						currentToolUseBlock = &ClaudeContentBlockToolUse{
							Type:  "tool_use",
							ID:    cb.ContentBlock.ID,
							Name:  cb.ContentBlock.Name,
							Input: map[string]any{},
						}
						currentToolInputBuilder.Reset()
					} else {
						currentToolUseBlock = nil
						currentToolInputBuilder.Reset()
					}
					currentTextBlock = nil
				}
			}
		case "content_block_delta":
			var d struct {
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text,omitempty"`
					PartialJSON string `json:"partial_json,omitempty"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(event.Data, &d); err == nil {
				switch d.Delta.Type {
				case "text_delta":
					if currentTextBlock != nil {
						currentTextBlock.Text += d.Delta.Text
					} else {
						textBlock := &ClaudeContentBlockText{Type: "text", Text: d.Delta.Text}
						contentBlocks = append(contentBlocks, textBlock)
						currentTextBlock = textBlock
					}
				case "input_json_delta":
					if currentToolUseBlock != nil {
						currentToolInputBuilder.WriteString(d.Delta.PartialJSON)
					}
				}
			}
		case "content_block_stop":
			if currentToolUseBlock != nil {
				inputStr := currentToolInputBuilder.String()
				var input map[string]any
				if inputStr != "" {
					if err := json.Unmarshal([]byte(inputStr), &input); err != nil {
						input = map[string]any{"raw": inputStr}
					}
				} else {
					input = map[string]any{}
				}
				currentToolUseBlock.Input = input
				toolUseBlocks = append(toolUseBlocks, currentToolUseBlock)
				currentToolUseBlock = nil
				currentToolInputBuilder.Reset()
			}
			currentTextBlock = nil
		case "message_delta":
			var d struct {
				Delta struct {
					StopReason   *string `json:"stop_reason"`
					StopSequence *string `json:"stop_sequence"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(event.Data, &d); err == nil {
				stopReason = d.Delta.StopReason
				stopSequence = d.Delta.StopSequence
				if d.Usage.OutputTokens > 0 {
					usage.OutputTokens = d.Usage.OutputTokens
				}
			}
		case "message_stop":
			// done
		}
	}

	var filteredContentBlocks []any
	for _, block := range contentBlocks {
		if textBlock, ok := block.(*ClaudeContentBlockText); ok {
			if textBlock.Text == "" {
				continue
			}
		}
		filteredContentBlocks = append(filteredContentBlocks, block)
	}
	for _, tub := range toolUseBlocks {
		filteredContentBlocks = append(filteredContentBlocks, tub)
	}

	resp = ClaudeMessagesResponse{
		ID:           id,
		Model:        model,
		Role:         "assistant",
		Content:      filteredContentBlocks,
		Type:         "message",
		StopReason:   stopReason,
		StopSequence: stopSequence,
		Usage:        usage,
	}
	return resp, nil
}

// eventStreamStripper is an io.Reader that strips "data: " prefixes and blank lines from an event stream.
type eventStreamStripper struct {
	r   io.Reader
	buf []byte
}

func (e *eventStreamStripper) Read(p []byte) (int, error) {
	if len(e.buf) == 0 {
		// Read a line from the underlying reader
		var lineBuf [4096]byte
		n, err := e.r.Read(lineBuf[:])
		if n == 0 && err != nil {
			return 0, err
		}
		lines := strings.Split(string(lineBuf[:n]), "\n")
		var out []byte
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				line = line[6:]
			}
			out = append(out, line...)
		}
		e.buf = out
	}
	n := copy(p, e.buf)
	e.buf = e.buf[n:]
	return n, nil
}

func ConvertClaudeToOAI(req ClaudeMessagesRequest) (OAIRequest, error) {
	var oaiReq OAIRequest
	oaiReq.Model = "gpt-4.1"
	if strings.Contains(req.Model, "haiku") {
		oaiReq.Model = "gpt-4o-mini"
	}
	oaiReq.MaxTokens = req.MaxTokens
	oaiReq.Temperature = req.Temperature
	oaiReq.TopP = req.TopP
	oaiReq.TopK = req.TopK
	oaiReq.Stream = true

	// Convert stop_sequences to stop
	if req.StopSequences != nil {
		oaiReq.Stop = req.StopSequences
	}

	// Convert Claude messages to OAI messages
	for _, cm := range req.Messages {
		var oaiContents []OAIMessageContent

		// Handle tool_result blocks in user messages as plain text, as in server.py
		if cm.Role == "user" {
			switch content := cm.Content.(type) {
			case []ClaudeContentBlockText:
				for _, block := range content {
					oaiContents = append(oaiContents, OAIMessageContent{
						Type: "text",
						Text: block.Text,
					})
				}
			case []any:
				// If any block is type tool_result, flatten to text
				hasToolResult := false
				var textContent strings.Builder
				for _, block := range content {
					switch b := block.(type) {
					case ClaudeContentBlockToolResult:
						hasToolResult = true
						toolID := b.ToolUseID
						textContent.WriteString(fmt.Sprintf("Tool Result for %s:\n", toolID))
						switch rc := b.Content.(type) {
						case []ClaudeContentBlockText:
							for _, item := range rc {
								textContent.WriteString(item.Text + "\n")
							}
						case string:
							textContent.WriteString(rc + "\n")
						default:
							bb, _ := json.Marshal(rc)
							textContent.WriteString(string(bb) + "\n")
						}
					case ClaudeContentBlockText:
						textContent.WriteString(b.Text + "\n")
					default:
						// fallback for legacy map[string]any
						if blockMap, ok := b.(map[string]any); ok {
							if blockMap["type"] == "tool_result" {
								hasToolResult = true
								toolID := ""
								if v, ok := blockMap["tool_use_id"].(string); ok {
									toolID = v
								}
								textContent.WriteString(fmt.Sprintf("Tool Result for %s:\n", toolID))
								resultContent := blockMap["content"]
								switch rc := resultContent.(type) {
								case []any:
									for _, item := range rc {
										if itemMap, ok := item.(map[string]any); ok {
											if itemMap["type"] == "text" {
												if s, ok := itemMap["text"].(string); ok {
													textContent.WriteString(s + "\n")
												}
											} else if s, ok := itemMap["text"].(string); ok {
												textContent.WriteString(s + "\n")
											} else {
												b, _ := json.Marshal(itemMap)
												textContent.WriteString(string(b) + "\n")
											}
										} else if s, ok := item.(string); ok {
											textContent.WriteString(s + "\n")
										} else {
											b, _ := json.Marshal(item)
											textContent.WriteString(string(b) + "\n")
										}
									}
								case map[string]any:
									if rc["type"] == "text" {
										if s, ok := rc["text"].(string); ok {
											textContent.WriteString(s + "\n")
										}
									} else {
										b, _ := json.Marshal(rc)
										textContent.WriteString(string(b) + "\n")
									}
								case string:
									textContent.WriteString(rc + "\n")
								default:
									b, _ := json.Marshal(rc)
									textContent.WriteString(string(b) + "\n")
								}
							} else if blockMap["type"] == "text" {
								if s, ok := blockMap["text"].(string); ok {
									textContent.WriteString(s + "\n")
								}
							}
						}
					}
				}
				if hasToolResult {
					oaiContents = append(oaiContents, OAIMessageContent{
						Type: "text",
						Text: strings.TrimSpace(textContent.String()),
					})
				} else {
					// Fallback to normal handling
					for _, block := range content {
						if b, ok := block.(ClaudeContentBlockText); ok {
							oaiContents = append(oaiContents, OAIMessageContent{
								Type: "text",
								Text: b.Text,
							})
						} else if blockMap, ok := block.(map[string]any); ok {
							if blockMap["type"] == "text" {
								if s, ok := blockMap["text"].(string); ok {
									oaiContents = append(oaiContents, OAIMessageContent{
										Type: "text",
										Text: s,
									})
								}
							}
						}
					}
				}
			case string:
				oaiContents = append(oaiContents, OAIMessageContent{
					Type: "text",
					Text: content,
				})
			default:
				// Fallback: marshal to string
				b, _ := json.Marshal(content)
				oaiContents = append(oaiContents, OAIMessageContent{
					Type: "text",
					Text: string(b),
				})
			}
		} else {
			// For assistant and other roles, preserve text blocks, ignore tool_use/tool_result
			switch content := cm.Content.(type) {
			case string:
				oaiContents = append(oaiContents, OAIMessageContent{
					Type: "text",
					Text: content,
				})
			case []ClaudeContentBlockText:
				for _, block := range content {
					oaiContents = append(oaiContents, OAIMessageContent{
						Type: "text",
						Text: block.Text,
					})
				}
			case []any:
				for _, block := range content {
					if b, ok := block.(ClaudeContentBlockText); ok {
						oaiContents = append(oaiContents, OAIMessageContent{
							Type: "text",
							Text: b.Text,
						})
					} else if blockMap, ok := block.(map[string]any); ok {
						if blockMap["type"] == "text" {
							if s, ok := blockMap["text"].(string); ok {
								oaiContents = append(oaiContents, OAIMessageContent{
									Type: "text",
									Text: s,
								})
							}
						}
					}
				}
			default:
				b, _ := json.Marshal(content)
				oaiContents = append(oaiContents, OAIMessageContent{
					Type: "text",
					Text: string(b),
				})
			}
		}

		// If content is empty or null, set to "..." to avoid nulls in OAI
		if len(oaiContents) == 0 {
			continue
		}

		oaiReq.Messages = append(oaiReq.Messages, OAIMessage{
			Role:    cm.Role,
			Content: oaiContents,
		})
	}

	// Convert tools to OAI function tools
	if req.Tools != nil {
		var oaiTools []OAIFunctionTool
		for _, t := range *req.Tools {
			oaiTools = append(oaiTools, OAIFunctionTool{
				Type: "function",
				Function: map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.InputSchema,
				},
			})
		}
		oaiReq.Tools = &oaiTools
	}

	// ToolChoice conversion
	if req.ToolChoice != nil {
		oaiReq.ToolChoice = ConvertToolChoiceClaudeToOAI(*req.ToolChoice)
	}

	return oaiReq, nil
}

// ConvertOAIToClaude converts an OAIRequest to a ClaudeMessagesRequest.
func ConvertOAIToClaude(req OAIRequest) (ClaudeMessagesRequest, error) {
	var claudeReq ClaudeMessagesRequest
	claudeReq.Model = req.Model
	claudeReq.MaxTokens = req.MaxTokens
	claudeReq.Temperature = req.Temperature
	claudeReq.TopP = req.TopP
	claudeReq.TopK = req.TopK
	claudeReq.Stream = &req.Stream

	// Convert stop to stop_sequences
	if req.Stop != nil {
		claudeReq.StopSequences = req.Stop
	}

	// Convert OAI messages to Claude messages
	for _, om := range req.Messages {
		claudeMsg := ClaudeMessage{
			Role: om.Role,
		}
		// Convert OAI content array to Claude content blocks
		var blocks []ClaudeContentBlockText
		for _, c := range om.Content {
			if c.Type == "text" {
				blocks = append(blocks, ClaudeContentBlockText{
					Type: "text",
					Text: c.Text,
				})
			}
			// Add more type handling if needed
		}
		claudeMsg.Content = blocks
		claudeReq.Messages = append(claudeReq.Messages, claudeMsg)
	}

	// Convert OAI tools to Claude tools
	if req.Tools != nil {
		var claudeTools []ClaudeTool
		for _, t := range *req.Tools {
			fn, ok := t.Function["name"].(string)
			if !ok {
				continue
			}
			desc, _ := t.Function["description"].(string)
			params, _ := t.Function["parameters"].(map[string]any)
			claudeTools = append(claudeTools, ClaudeTool{
				Name:        fn,
				Description: &desc,
				InputSchema: params,
			})
		}
		claudeReq.Tools = &claudeTools
	}

	// ToolChoice direct mapping (must be *map[string]any or nil)
	if req.ToolChoice != nil {
		if tc, ok := req.ToolChoice.(map[string]any); ok {
			claudeReq.ToolChoice = &tc
		} else {
			claudeReq.ToolChoice = nil
		}
	}

	return claudeReq, nil
}

// StreamEvent represents a single event in the Claude streaming protocol.
type StreamEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// ClaudeStreamChunk is a generic struct for Claude streaming events.
type ClaudeStreamChunk struct {
	Type         string         `json:"type"`
	Message      map[string]any `json:"message,omitempty"`
	Index        int            `json:"index,omitempty"`
	ContentBlock map[string]any `json:"content_block,omitempty"`
	Delta        map[string]any `json:"delta,omitempty"`
	Usage        map[string]any `json:"usage,omitempty"`
}

// OAIStreamChunk is a generic struct for OpenAI streaming events.
type OAIStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   string        `json:"content,omitempty"`
			ToolCalls []OAIToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type OAIToolCall struct {
	Function OAIToolCallFunction `json:"function"`
	Id       string              `json:"id"`
	Index    int                 `json:"index"`
	Type     string              `json:"type"`
}
type OAIToolCallFunction struct {
	Arguments string `json:"arguments"`
	Name      string `json:"name"`
}

// ConvertOAIStreamToClaudeStream reads OpenAI streaming chunks from r, converts them to Claude streaming events, and writes to w.
func ConvertOAIStreamToClaudeStream(r io.Reader, w io.Writer, model string) error {
	encoder := json.NewEncoder(w)

	// Send message_start event
	messageID := fmt.Sprintf("msg_%024x", 0)
	messageStart := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":                0,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
				"output_tokens":               0,
			},
		},
	}
	encoder.Encode(map[string]any{"event": "message_start", "data": messageStart})

	// Send content_block_start for text
	encoder.Encode(map[string]any{
		"event": "content_block_start",
		"data": map[string]any{
			"type":          "content_block_start",
			"index":         0,
			"content_block": map[string]any{"type": "text", "text": ""},
		},
	})

	// Send ping event
	encoder.Encode(map[string]any{"event": "ping", "data": map[string]any{"type": "ping"}})

	var accumulatedText string
	var textBlockClosed bool
	var outputTokens int

	// Read line by line, strip "data: ", skip empty lines, stop at [DONE]
	bufReader := io.Reader(r)
	lineReader := bufio.NewReader(bufReader)
	lastToolIndex := -1
	for {
		line, err := lineReader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if err == io.EOF {
				break
			}
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			if err == io.EOF {
				break
			}
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk OAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// skip invalid lines, but log for debugging
			fmt.Fprintf(w, "data: {\"error\": \"invalid chunk: %s\"}\n\n", err.Error())
			if err == io.EOF {
				break
			}
			continue
		}

		for _, choice := range chunk.Choices {
			// Handle tool calls (OpenAI tool_calls in delta)
			if len(choice.Delta.ToolCalls) > 0 {
				// Close text block if open
				if !textBlockClosed {
					encoder.Encode(map[string]any{
						"event": "content_block_stop",
						"data": map[string]any{
							"type":  "content_block_stop",
							"index": 0,
						},
					})
					textBlockClosed = true
				}
				for _, toolCall := range choice.Delta.ToolCalls {
					// Start tool_use block
					if lastToolIndex != toolCall.Index {
						if lastToolIndex == -1 {
							// Stop tool_use block
							encoder.Encode(map[string]any{
								"event": "content_block_stop",
								"data": map[string]any{
									"type":  "content_block_stop",
									"index": lastToolIndex,
								},
							})
						}
						lastToolIndex = toolCall.Index
						encoder.Encode(map[string]any{
							"event": "content_block_start",
							"data": map[string]any{
								"type":  "content_block_start",
								"index": toolCall.Index,
								"content_block": map[string]any{
									"type":  "tool_use",
									"id":    toolCall.Id,
									"name":  toolCall.Function.Name,
									"input": map[string]any{},
								},
							},
						})
					}
					// Send input_json_delta
					encoder.Encode(map[string]any{
						"event": "content_block_delta",
						"data": map[string]any{
							"type":  "content_block_delta",
							"index": toolCall.Index,
							"delta": map[string]any{
								"type":         "input_json_delta",
								"partial_json": toolCall.Function.Arguments,
								"index":        toolCall.Index,
							},
						},
					})

				}
			}

			// Handle text deltas
			if choice.Delta.Content != "" && !textBlockClosed {
				accumulatedText += choice.Delta.Content
				encoder.Encode(map[string]any{
					"event": "content_block_delta",
					"data": map[string]any{
						"type":  "content_block_delta",
						"index": 0,
						"delta": map[string]any{
							"type": "text_delta",
							"text": choice.Delta.Content,
						},
					},
				})
			}

			// Handle finish_reason
			if choice.FinishReason != nil {
				if lastToolIndex != -1 {
					// Stop tool_use block
					encoder.Encode(map[string]any{
						"event": "content_block_stop",
						"data": map[string]any{
							"type":  "content_block_stop",
							"index": lastToolIndex,
						},
					})
				}
				if !textBlockClosed {
					textBlockClosed = true
					encoder.Encode(map[string]any{
						"event": "content_block_stop",
						"data": map[string]any{
							"type":  "content_block_stop",
							"index": 0,
						},
					})
				}

				stopReason := "end_turn"
				switch *choice.FinishReason {
				case "length":
					stopReason = "max_tokens"
				case "tool_calls":
					stopReason = "tool_use"
				case "stop":
					stopReason = "end_turn"
				}

				encoder.Encode(map[string]any{
					"event": "message_delta",
					"data": map[string]any{
						"type": "message_delta",
						"delta": map[string]any{
							"stop_reason":   stopReason,
							"stop_sequence": nil,
						},
						"usage": map[string]any{
							"output_tokens": outputTokens,
						},
					},
				})

				encoder.Encode(map[string]any{
					"event": "message_stop",
					"data":  map[string]any{"type": "message_stop"},
				})

				// Send [DONE] marker
				fmt.Fprint(w, "data: [DONE]\n\n")
				return nil
			}
		}
		if err == io.EOF {
			break
		}
	}

	// If we never saw a finish_reason, close the text block and send message_stop
	if !textBlockClosed {
		encoder.Encode(map[string]any{
			"event": "content_block_stop",
			"data": map[string]any{
				"type":  "content_block_stop",
				"index": 0,
			},
		})
		encoder.Encode(map[string]any{
			"event": "message_delta",
			"data": map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason":   "end_turn",
					"stop_sequence": nil,
				},
				"usage": map[string]any{
					"output_tokens": outputTokens,
				},
			},
		})
		encoder.Encode(map[string]any{
			"event": "message_stop",
			"data":  map[string]any{"type": "message_stop"},
		})
		fmt.Fprint(w, "data: [DONE]\n\n")
	}

	return nil
}
