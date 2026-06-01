//go:build integration

package llm

import (
	"context"
	"os"
	"testing"
)

func TestIntegration_Chat(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client := NewClient(apiKey, "gpt-4o")

	messages := []Message{
		{Role: RoleSystem, Content: "Return a JSON object with a 'name' field set to 'Alice' and an 'age' field set to 7."},
	}

	var result struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	if err := client.Chat(context.Background(), messages, &result, 0.1); err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if result.Name != "Alice" {
		t.Errorf("expected name=Alice, got %q", result.Name)
	}
	if result.Age != 7 {
		t.Errorf("expected age=7, got %d", result.Age)
	}
}

func TestIntegration_CharacterExtractionPrompt(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client := NewClient(apiKey, "gpt-4o")

	messages := []Message{
		{Role: RoleSystem, Content: SystemPromptExtractCharacters()},
		{Role: RoleUser, Content: "Cover: A story about Alice in Wonderland.\n\nChapter: Alice was sitting on the riverbank with her sister when a white rabbit with pink eyes ran past, checking a pocket watch."},
	}

	var result map[string]any
	if err := client.Chat(context.Background(), messages, &result, 0.1); err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if result["characters"] == nil {
		t.Error("expected characters in response")
	}
}

func TestIntegration_SceneExtractionPrompt(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client := NewClient(apiKey, "gpt-4o")

	messages := []Message{
		{Role: RoleSystem, Content: SystemPromptExtractScenes()},
		{Role: RoleUser, Content: "Chapter: Alice was sitting on the riverbank with her sister when a white rabbit ran past.\n\nCharacter Reference:\n{\"characters\":[{\"id\":\"alice\",\"name\":\"Alice\",\"physical_description\":\"Young girl\"},{\"id\":\"white_rabbit\",\"name\":\"White Rabbit\",\"physical_description\":\"White rabbit with pink eyes\"}]}"},
	}

	var result map[string]any
	if err := client.Chat(context.Background(), messages, &result, 0.1); err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if result["scenes"] == nil {
		t.Error("expected scenes in response")
	}
}
