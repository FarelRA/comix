package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"

	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Result struct {
	Content      string
	PromptTokens int
	TotalTokens  int
}

type Client struct {
	model        string
	apiKey       string
	thinking     string
	baseURL      string
	maxRetries   int
	httpClient   *http.Client
	once         sync.Once
	openaiClient openai.Client
}

func NewClient(apiKey, model, thinking string) *Client {
	return &Client{
		model:    model,
		apiKey:   apiKey,
		thinking: thinking,
	}
}

func (c *Client) getClient() *openai.Client {
	c.once.Do(func() {
		opts := []option.RequestOption{
			option.WithAPIKey(c.apiKey),
		}
		if c.baseURL != "" {
			opts = append(opts, option.WithBaseURL(c.baseURL))
		}
		if c.maxRetries > 0 {
			opts = append(opts, option.WithMaxRetries(c.maxRetries))
		}
		if c.httpClient != nil {
			opts = append(opts, option.WithHTTPClient(c.httpClient))
		}
		cl := openai.NewClient(opts...)
		c.openaiClient = cl
	})
	return &c.openaiClient
}

func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	c.httpClient = hc
	return c
}

func (c *Client) WithMaxRetries(n int) *Client {
	c.maxRetries = n
	return c
}

func (c *Client) WithBaseURL(url string) *Client {
	c.baseURL = url
	return c
}

func (c *Client) Chat(ctx context.Context, messages []Message, schema any, temperature float64) error {
	if schema == nil {
		return fmt.Errorf("llm chat: schema is required; use ChatRaw for unstructured responses")
	}

	apiMessages := buildMessages(messages)

	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(c.model),
		Messages:    apiMessages,
		Temperature: openai.Float(temperature),
	}
	params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONSchema: structuredResponseFormat(schema)}
	if c.thinking != "" {
		params.ReasoningEffort = shared.ReasoningEffort(c.thinking)
	}

	resp, err := c.getClient().Chat.Completions.New(ctx, params)
	if err != nil {
		return fmt.Errorf("llm chat: %w", err)
	}

	if len(resp.Choices) == 0 {
		return fmt.Errorf("llm returned no choices")
	}

	content := resp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), schema); err != nil {
		return fmt.Errorf("parsing llm response as json: %w\nresponse: %s", err, content)
	}

	return nil
}

func (c *Client) ChatRaw(ctx context.Context, messages []Message, temperature float64) (*Result, error) {
	apiMessages := buildMessages(messages)

	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(c.model),
		Messages:    apiMessages,
		Temperature: openai.Float(temperature),
	}
	if c.thinking != "" {
		params.ReasoningEffort = shared.ReasoningEffort(c.thinking)
	}

	resp, err := c.getClient().Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("llm chat: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	result := &Result{
		Content:      resp.Choices[0].Message.Content,
		PromptTokens: int(resp.Usage.PromptTokens),
		TotalTokens:  int(resp.Usage.TotalTokens),
	}

	return result, nil
}

func structuredResponseFormat(schema any) *shared.ResponseFormatJSONSchemaParam {
	schemaName := "response"
	if schema != nil {
		t := reflect.TypeOf(schema)
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Name() != "" {
			schemaName = t.Name()
		}
	}

	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	return &shared.ResponseFormatJSONSchemaParam{
		JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   schemaName,
			Strict: openai.Bool(true),
			Schema: reflector.Reflect(schema),
		},
	}
}

func buildMessages(messages []Message) []openai.ChatCompletionMessageParamUnion {
	apiMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, m := range messages {
		switch m.Role {
		case RoleSystem:
			apiMessages[i] = openai.SystemMessage(m.Content)
		case RoleUser:
			apiMessages[i] = openai.UserMessage(m.Content)
		case RoleAssistant:
			apiMessages[i] = openai.AssistantMessage(m.Content)
		}
	}
	return apiMessages
}
