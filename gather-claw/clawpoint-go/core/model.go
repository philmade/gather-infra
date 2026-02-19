package core

import (
	"context"
	"fmt"
	"log"
	"os"

	"clawpoint-go/anthropicmodel"

	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"
)

// CreateModel builds an LLM client based on MODEL_PROVIDER env var.
func CreateModel(ctx context.Context) (model.LLM, error) {
	provider := os.Getenv("MODEL_PROVIDER")

	switch provider {
	case "anthropic":
		baseURL := os.Getenv("ANTHROPIC_API_BASE")
		if baseURL == "" {
			baseURL = "https://api.z.ai/api/anthropic"
		}
		modelName := os.Getenv("ANTHROPIC_MODEL")
		if modelName == "" {
			modelName = "glm-5"
		}
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is required")
		}
		return anthropicmodel.New(anthropicmodel.Config{
			Model:   modelName,
			BaseURL: baseURL,
			APIKey:  apiKey,
		}), nil

	default:
		apiKey := os.Getenv("GOOGLE_API_KEY")
		if apiKey == "" {
			log.Fatal("GOOGLE_API_KEY required when MODEL_PROVIDER is gemini")
		}
		modelName := os.Getenv("GEMINI_MODEL")
		if modelName == "" {
			modelName = "gemini-2.0-flash-exp"
		}
		return gemini.NewModel(ctx, modelName, &genai.ClientConfig{
			APIKey: apiKey,
		})
	}
}
