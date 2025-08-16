package providers

import (
	"fmt"
	"net/url"
	"strings"
)

// Provider interface defines the contract for all LLM providers
type Provider interface {
	Name() string
	SupportsStreaming() bool
	TransformRequest(request []byte) ([]byte, error)
	TransformResponse(response []byte) ([]byte, error)
	TransformStream(chunk []byte, state *StreamState) ([]byte, error)
	IsStreaming(headers map[string][]string) bool
	GetEndpoint() string
	SetAPIKey(key string)
}

// StreamState tracks streaming conversion state
type StreamState struct {
	MessageStartSent bool
	MessageID        string
	Model            string
	InitialUsage     map[string]any

	// Content block tracking for multiple blocks (text, tool_use, etc.)
	ContentBlocks map[int]*ContentBlockState
	CurrentIndex  int
}

// ContentBlockState tracks individual content block state during streaming
type ContentBlockState struct {
	Type          string // "text" or "tool_use"
	StartSent     bool
	StopSent      bool
	ToolCallID    string // For tool_use blocks
	ToolCallIndex int    // OpenRouter tool call index for tracking across chunks
	ToolName      string // For tool_use blocks
	Arguments     string // Accumulated arguments for tool_use blocks
}

// Registry manages provider instances
type Registry struct {
	providers map[string]Provider
	domainMappings map[string]string
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry
func (r *Registry) Register(provider Provider) {
	r.providers[provider.Name()] = provider
}

// Get retrieves a provider by name
func (r *Registry) Get(name string) (Provider, bool) {
	provider, exists := r.providers[name]
	return provider, exists
}

func (r *Registry) SetDomainMappings(mappings map[string]string) {
    r.domainMappings = mappings
}

// GetByDomain returns a provider based on the API base URL domain
func (r *Registry) GetByDomain(apiBase string) (Provider, error) {
    u, err := url.Parse(apiBase)
    if err != nil {
        return nil, fmt.Errorf("invalid API base URL: %w", err)
    }

    domain := strings.ToLower(u.Hostname())

    // Check config-based mappings first
    if r.domainMappings != nil {
        if providerName, exists := r.domainMappings[domain]; exists {
            if provider, found := r.Get(providerName); found {
                return provider, nil
            }
        }
    }

    // Fall back to hardcoded mappings
    domainProviderMap := map[string]string{
		"openrouter.ai":                     "openrouter",
		"api.openrouter.ai":                 "openrouter",
		"api.openai.com":                    "openai",
		"openai.com":                        "openai",
		"api.anthropic.com":                 "anthropic",
		"anthropic.com":                     "anthropic",
		"integrate.api.nvidia.com":          "nvidia",
		"api.nvidia.com":                    "nvidia",
		"generativelanguage.googleapis.com": "gemini",
		"googleapis.com":                    "gemini",
	}

    if providerName, exists := domainProviderMap[domain]; exists {
        if provider, found := r.Get(providerName); found {
            return provider, nil
        }
    }

    return nil, fmt.Errorf("no provider found for domain: %s", domain)
}

// List returns all registered provider names
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}

	return names
}

// Initialize registers all built-in providers
func (r *Registry) Initialize() {
	r.Register(NewOpenRouterProvider())
	r.Register(NewOpenAIProvider())
	r.Register(NewAnthropicProvider())
	r.Register(NewNvidiaProvider())
	r.Register(NewGeminiProvider())
}
