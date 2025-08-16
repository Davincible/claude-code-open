package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPort           = 6970
	DefaultConfigFilename = "config.json"
	DefaultYAMLFilename   = "config.yaml"
	DefaultHost           = "127.0.0.1"
)

var (
	// Default provider URLs
	DefaultProviderURLs = map[string]string{
		"openrouter": "https://openrouter.ai/api/v1/chat/completions",
		"openai":     "https://api.openai.com/v1/chat/completions",
		"anthropic":  "https://api.anthropic.com/v1/messages",
		"nvidia":     "https://integrate.api.nvidia.com/v1/chat/completions",
		"gemini":     "https://generativelanguage.googleapis.com/v1beta/models",
	}

	// Default models for each provider
	DefaultProviderModels = map[string][]string{
		"openrouter": {
			"anthropic/claude-3.5-sonnet",
			"anthropic/claude-3-opus",
			"openai/gpt-4-turbo",
			"openai/gpt-4o",
		},
		"openai": {
			"gpt-4o",
			"gpt-4-turbo",
			"gpt-4",
			"gpt-3.5-turbo",
		},
		"anthropic": {
			"claude-3-5-sonnet-20241022",
			"claude-3-opus-20240229",
			"claude-3-haiku-20240307",
		},
		"nvidia": {
			"nvidia/llama-3.1-nemotron-70b-instruct",
			"nvidia/llama-3.1-nemotron-51b-instruct",
		},
		"gemini": {
			"gemini-2.0-flash",
			"gemini-1.5-pro",
			"gemini-1.5-flash",
		},
	}
)

type Provider struct {
	Name           string   `json:"name" yaml:"name"`
	APIBase        string   `json:"api_base_url" yaml:"url,omitempty"`
	APIKey         string   `json:"api_key" yaml:"api_key,omitempty"`
	Models         []string `json:"models" yaml:"models,omitempty"`
	ModelWhitelist []string `json:"model_whitelist,omitempty" yaml:"model_whitelist,omitempty"`
	DefaultModels  []string `json:"default_models,omitempty" yaml:"default_models,omitempty"`
}

type RouterConfig struct {
	Default     string `json:"default" yaml:"default,omitempty"`
	Think       string `json:"think,omitempty" yaml:"think,omitempty"`
	Background  string `json:"background,omitempty" yaml:"background,omitempty"`
	LongContext string `json:"longContext,omitempty" yaml:"long_context,omitempty"`
	WebSearch   string `json:"webSearch,omitempty" yaml:"web_search,omitempty"`
}

type Config struct {
    Host           string            `json:"HOST,omitempty" yaml:"host,omitempty"`
    Port           int               `json:"PORT,omitempty" yaml:"port,omitempty"`
    APIKey         string            `json:"APIKEY,omitempty" yaml:"api_key,omitempty"`
    Providers      []Provider        `json:"Providers" yaml:"providers"`
    Router         RouterConfig      `json:"Router" yaml:"router,omitempty"`
    DomainMappings map[string]string `json:"domain_mappings,omitempty" yaml:"domain_mappings,omitempty"`
}


type Manager struct {
	baseDir     string
	jsonPath    string
	yamlPath    string
	configValue atomic.Value
}

func NewManager(baseDir string) *Manager {
	return &Manager{
		baseDir:  baseDir,
		jsonPath: filepath.Join(baseDir, DefaultConfigFilename),
		yamlPath: filepath.Join(baseDir, DefaultYAMLFilename),
	}
}

// createMinimalConfig creates a minimal configuration with all providers using CCO_API_KEY
func (m *Manager) createMinimalConfig() Config {
	return Config{
		Host: DefaultHost,
		Port: DefaultPort,
		Providers: []Provider{
			{Name: "openrouter"},
			{Name: "openai"},
			{Name: "anthropic"},
			{Name: "nvidia"},
			{Name: "gemini"},
		},
		Router: RouterConfig{
			Default:     "openrouter,anthropic/claude-3.5-sonnet",
			Think:       "openai,o1-preview",
			Background:  "anthropic,claude-3-haiku-20240307",
			LongContext: "anthropic,claude-3-5-sonnet-20241022",
			WebSearch:   "openrouter,perplexity/llama-3.1-sonar-huge-128k-online",
		},
	}
}

func (m *Manager) Load() (*Config, error) {
	var (
		cfg Config
		err error
	)

	// Check if CCO_API_KEY is set - if so, we can run without a config file
	ccoAPIKey := os.Getenv("CCO_API_KEY")

	// Try YAML first (takes precedence)
	if _, yamlErr := os.Stat(m.yamlPath); yamlErr == nil {
		cfg, err = m.loadYAML()
		if err != nil {
			return nil, fmt.Errorf("load YAML config: %w", err)
		}
	} else if _, jsonErr := os.Stat(m.jsonPath); jsonErr == nil {
		cfg, err = m.loadJSON()
		if err != nil {
			return nil, fmt.Errorf("load JSON config: %w", err)
		}
	} else if ccoAPIKey != "" {
		// No config file found, but CCO_API_KEY is set - create minimal config
		cfg = m.createMinimalConfig()
	} else {
		return nil, fmt.Errorf("no configuration file found (looked for %s or %s) and CCO_API_KEY environment variable not set", m.yamlPath, m.jsonPath)
	}

	// Apply defaults and validation
	m.applyDefaults(&cfg)

	m.configValue.Store(&cfg)

	return &cfg, nil
}

func (m *Manager) loadYAML() (Config, error) {
	var cfg Config

	data, err := os.ReadFile(m.yamlPath)
	if err != nil {
		return cfg, fmt.Errorf("read YAML config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal YAML config: %w", err)
	}

	return cfg, nil
}

func (m *Manager) loadJSON() (Config, error) {
	var cfg Config

	data, err := os.ReadFile(m.jsonPath)
	if err != nil {
		return cfg, fmt.Errorf("read JSON config file: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal JSON config: %w", err)
	}

	return cfg, nil
}

func (m *Manager) applyDefaults(cfg *Config) {
	// Set basic defaults
	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}

	if cfg.Host == "" {
		cfg.Host = DefaultHost
	}

	// Apply provider defaults
	for i := range cfg.Providers {
		provider := &cfg.Providers[i]

		// Set default URL if not provided
		if provider.APIBase == "" {
			if defaultURL, exists := DefaultProviderURLs[provider.Name]; exists {
				provider.APIBase = defaultURL
			}
		}

		// Set default models if not provided
		if len(provider.DefaultModels) == 0 {
			if defaultModels, exists := DefaultProviderModels[provider.Name]; exists {
				provider.DefaultModels = make([]string, len(defaultModels))
				copy(provider.DefaultModels, defaultModels)
			}
		}

		// Validate model whitelist against default models if provided
		if len(provider.ModelWhitelist) > 0 && len(provider.DefaultModels) > 0 {
			// Filter default models based on whitelist
			var filteredDefaults []string

			for _, model := range provider.DefaultModels {
				for _, whitelisted := range provider.ModelWhitelist {
					if strings.Contains(model, whitelisted) || model == whitelisted {
						filteredDefaults = append(filteredDefaults, model)
						break
					}
				}
			}

			provider.DefaultModels = filteredDefaults
		}
	}
}

func (m *Manager) Get() *Config {
	if v := m.configValue.Load(); v != nil {
		if cfg, ok := v.(*Config); ok {
			return cfg
		}
	}

	cfg, err := m.Load()
	if err != nil {
		// Return a config with defaults if loading fails
		return &Config{
			Host: DefaultHost,
			Port: DefaultPort,
		}
	}

	return cfg
}

func (m *Manager) Save(cfg *Config) error {
	if err := os.MkdirAll(m.baseDir, 0750); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Prefer YAML format for new saves
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal YAML config: %w", err)
	}

	if err := os.WriteFile(m.yamlPath, data, 0600); err != nil {
		return fmt.Errorf("write YAML config file: %w", err)
	}

	m.configValue.Store(cfg)

	return nil
}

func (m *Manager) SaveAsYAML(cfg *Config) error {
	if err := os.MkdirAll(m.baseDir, 0750); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal YAML config: %w", err)
	}

	if err := os.WriteFile(m.yamlPath, data, 0600); err != nil {
		return fmt.Errorf("write YAML config file: %w", err)
	}

	m.configValue.Store(cfg)

	return nil
}

func (m *Manager) SaveAsJSON(cfg *Config) error {
	if err := os.MkdirAll(m.baseDir, 0750); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON config: %w", err)
	}

	if err := os.WriteFile(m.jsonPath, data, 0600); err != nil {
		return fmt.Errorf("write JSON config file: %w", err)
	}

	m.configValue.Store(cfg)

	return nil
}

func (m *Manager) GetPath() string {
	// Return YAML path if it exists, otherwise JSON path
	if _, err := os.Stat(m.yamlPath); err == nil {
		return m.yamlPath
	}

	return m.jsonPath
}

func (m *Manager) GetYAMLPath() string {
	return m.yamlPath
}

func (m *Manager) GetJSONPath() string {
	return m.jsonPath
}

func (m *Manager) Exists() bool {
	_, yamlErr := os.Stat(m.yamlPath)
	_, jsonErr := os.Stat(m.jsonPath)

	return yamlErr == nil || jsonErr == nil
}

func (m *Manager) HasYAML() bool {
	_, err := os.Stat(m.yamlPath)
	return err == nil
}

func (m *Manager) HasJSON() bool {
	_, err := os.Stat(m.jsonPath)
	return err == nil
}

// CreateExampleYAML creates an example YAML configuration with all available providers
func (m *Manager) CreateExampleYAML() error {
    cfg := &Config{
        Host:   DefaultHost,
        Port:   DefaultPort,
        APIKey: "your-proxy-api-key-here",
        Providers: []Provider{
            {
                Name:           "openrouter",
                APIKey:         "your-openrouter-api-key",
                ModelWhitelist: []string{"claude", "gpt-4"},
            },
            {
                Name:    "local-lmstudio",
                APIBase: "http://localhost:1234/v1/chat/completions",
                APIKey:  "not-needed",
            },
            {
                Name:   "openai",
                APIKey: "your-openai-api-key",
            },
            {
                Name:   "anthropic",
                APIKey: "your-anthropic-api-key",
            },
            {
                Name:   "nvidia",
                APIKey: "your-nvidia-api-key",
            },
            {
                Name:   "gemini",
                APIKey: "your-gemini-api-key",
            },
        },
        DomainMappings: map[string]string{
            "localhost": "openai",
            "127.0.0.1": "gemini",
            "0.0.0.0":   "openrouter",
        },
        Router: RouterConfig{
            Default:     "local-lmstudio,qwen/qwen3-coder-30b",
            Think:       "openai,o1-preview",
            Background:  "anthropic,claude-3-haiku-20240307",
            LongContext: "anthropic,claude-3-5-sonnet-20241022",
            WebSearch:   "openrouter,perplexity/llama-3.1-sonar-huge-128k-online",
        },
    }

    // Apply defaults to populate URLs and default models
    m.applyDefaults(cfg)

    return m.SaveAsYAML(cfg)
}


// IsModelAllowed checks if a model is allowed based on the provider's whitelist
func (p *Provider) IsModelAllowed(model string) bool {
	// If no whitelist is defined, all models are allowed
	if len(p.ModelWhitelist) == 0 {
		return true
	}

	// Check if model matches any whitelist entry
	for _, whitelisted := range p.ModelWhitelist {
		if strings.Contains(model, whitelisted) || model == whitelisted {
			return true
		}
	}

	return false
}

// GetAllowedModels returns all models that are allowed based on the whitelist
func (p *Provider) GetAllowedModels() []string {
	if len(p.ModelWhitelist) == 0 {
		return p.DefaultModels
	}

	var allowed []string

	for _, model := range p.DefaultModels {
		if p.IsModelAllowed(model) {
			allowed = append(allowed, model)
		}
	}

	return allowed
}
