package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_YAML_Support(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir)

	// Test YAML configuration loading
	yamlConfig := `
host: "0.0.0.0"
port: 8080
api_key: "test-proxy-key"
providers:
  - name: "openrouter"
    api_key: "test-openrouter-key"
    model_whitelist: ["claude", "gpt-4"]
  - name: "openai"
    api_key: "test-openai-key"
    url: "https://api.openai.com/v1/chat/completions"
router:
  default: "openrouter/anthropic/claude-3.5-sonnet"
  think: "openai/o1-preview"
`

	yamlPath := filepath.Join(tempDir, DefaultYAMLFilename)
	err := os.WriteFile(yamlPath, []byte(yamlConfig), 0644)
	require.NoError(t, err)

	cfg, err := mgr.Load()
	require.NoError(t, err)

	// Test basic config values
	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "test-proxy-key", cfg.APIKey)

	// Test providers
	require.Len(t, cfg.Providers, 2)

	openrouter := cfg.Providers[0]
	assert.Equal(t, "openrouter", openrouter.Name)
	assert.Equal(t, "test-openrouter-key", openrouter.APIKey)
	assert.Equal(t, DefaultProviderURLs["openrouter"], openrouter.APIBase) // Should be set from defaults
	assert.Equal(t, []string{"claude", "gpt-4"}, openrouter.ModelWhitelist)
	assert.NotEmpty(t, openrouter.DefaultModels) // Should be populated from defaults

	openai := cfg.Providers[1]
	assert.Equal(t, "openai", openai.Name)
	assert.Equal(t, "test-openai-key", openai.APIKey)
	assert.Equal(t, "https://api.openai.com/v1/chat/completions", openai.APIBase)

	// Test router config
	assert.Equal(t, "openrouter/anthropic/claude-3.5-sonnet", cfg.Router.Default)
	assert.Equal(t, "openai/o1-preview", cfg.Router.Think)
}

func TestManager_YAML_Takes_Precedence(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir)

	// Create both JSON and YAML configs
	jsonConfig := `{
		"HOST": "127.0.0.1",
		"PORT": 6970,
		"Providers": [
			{
				"name": "openai",
				"api_key": "json-key"
			}
		]
	}`

	yamlConfig := `
host: "0.0.0.0"
port: 8080
providers:
  - name: "openrouter" 
    api_key: "yaml-key"
`

	jsonPath := filepath.Join(tempDir, DefaultConfigFilename)
	yamlPath := filepath.Join(tempDir, DefaultYAMLFilename)

	err := os.WriteFile(jsonPath, []byte(jsonConfig), 0644)
	require.NoError(t, err)

	err = os.WriteFile(yamlPath, []byte(yamlConfig), 0644)
	require.NoError(t, err)

	cfg, err := mgr.Load()
	require.NoError(t, err)

	// Should use YAML values
	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "openrouter", cfg.Providers[0].Name)
	assert.Equal(t, "yaml-key", cfg.Providers[0].APIKey)
}

func TestManager_SaveAsYAML(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir)

	cfg := &Config{
		Host:   "127.0.0.1",
		Port:   7000,
		APIKey: "test-key",
		Providers: []Provider{
			{
				Name:           "openrouter",
				APIKey:         "test-openrouter-key",
				ModelWhitelist: []string{"claude", "gpt-4"},
			},
		},
		Router: RouterConfig{
			Default: "openrouter/anthropic/claude-3.5-sonnet",
		},
	}

	err := mgr.SaveAsYAML(cfg)
	require.NoError(t, err)

	// Verify file was created
	yamlPath := filepath.Join(tempDir, DefaultYAMLFilename)
	assert.FileExists(t, yamlPath)

	// Load and verify content
	loadedCfg, err := mgr.Load()
	require.NoError(t, err)

	assert.Equal(t, cfg.Host, loadedCfg.Host)
	assert.Equal(t, cfg.Port, loadedCfg.Port)
	assert.Equal(t, cfg.APIKey, loadedCfg.APIKey)
	assert.Equal(t, cfg.Providers[0].Name, loadedCfg.Providers[0].Name)
	assert.Equal(t, cfg.Providers[0].APIKey, loadedCfg.Providers[0].APIKey)
	assert.Equal(t, cfg.Providers[0].ModelWhitelist, loadedCfg.Providers[0].ModelWhitelist)
}

func TestManager_CreateExampleYAML(t *testing.T) {
    tempDir := t.TempDir()
    mgr := NewManager(tempDir)

    err := mgr.CreateExampleYAML()
    require.NoError(t, err)

    cfg, err := mgr.Load()
    require.NoError(t, err)

    // Update expectation to 6 providers (including local-lmstudio)
    require.Len(t, cfg.Providers, 6, "Expected 6 providers including local-lmstudio")

    // Test that domain mappings are included in the example
    require.NotNil(t, cfg.DomainMappings)
    assert.NotEmpty(t, cfg.DomainMappings, "Domain mappings should be included in example")

    // Check each provider has required fields
    providerNames := make(map[string]Provider)
    for _, p := range cfg.Providers {
        assert.NotEmpty(t, p.Name, "Provider name should not be empty")
        assert.NotEmpty(t, p.APIKey, "Provider API key should not be empty")
        providerNames[p.Name] = p
    }

    // Verify specific providers exist
    expectedProviders := []string{"openrouter", "local-lmstudio", "openai", "anthropic", "nvidia", "gemini"}
    for _, expected := range expectedProviders {
        _, exists := providerNames[expected]
        assert.True(t, exists, "Provider %s should exist in example config", expected)
    }

    // Check that non-local providers have default models
    for name, provider := range providerNames {
        if name == "local-lmstudio" {
            // Local providers don't need default models
            continue
        }
        assert.NotEmpty(t, provider.DefaultModels, 
            "Provider %s should have default models", name)
    }

    // Test domain mappings have expected entries
    expectedMappings := map[string]string{
        "localhost": "openai",
        "127.0.0.1": "gemini",
        "0.0.0.0":   "openrouter",
    }
    
    for domain, expectedProvider := range expectedMappings {
        actualProvider, exists := cfg.DomainMappings[domain]
        assert.True(t, exists, "Domain mapping for %s should exist", domain)
        assert.Equal(t, expectedProvider, actualProvider, 
            "Domain %s should map to %s", domain, expectedProvider)
    }
}


func TestProvider_ModelWhitelist(t *testing.T) {
	provider := Provider{
		Name:           "openrouter",
		ModelWhitelist: []string{"claude", "gpt-4"},
		DefaultModels: []string{
			"anthropic/claude-3.5-sonnet",
			"anthropic/claude-3-opus",
			"openai/gpt-4-turbo",
			"openai/gpt-3.5-turbo",
			"meta-llama/llama-3.1-70b",
		},
	}

	// Test allowed models
	assert.True(t, provider.IsModelAllowed("anthropic/claude-3.5-sonnet"))
	assert.True(t, provider.IsModelAllowed("openai/gpt-4-turbo"))
	assert.False(t, provider.IsModelAllowed("meta-llama/llama-3.1-70b"))
	assert.False(t, provider.IsModelAllowed("openai/gpt-3.5-turbo"))

	// Test getting allowed models
	allowed := provider.GetAllowedModels()
	expected := []string{
		"anthropic/claude-3.5-sonnet",
		"anthropic/claude-3-opus",
		"openai/gpt-4-turbo",
	}
	assert.Equal(t, expected, allowed)
}

func TestProvider_NoWhitelist(t *testing.T) {
	provider := Provider{
		Name: "openai",
		DefaultModels: []string{
			"gpt-4o",
			"gpt-4-turbo",
			"gpt-3.5-turbo",
		},
		// No ModelWhitelist - all models should be allowed
	}

	// All models should be allowed
	assert.True(t, provider.IsModelAllowed("gpt-4o"))
	assert.True(t, provider.IsModelAllowed("gpt-3.5-turbo"))
	assert.True(t, provider.IsModelAllowed("any-model"))

	// Should return all default models
	allowed := provider.GetAllowedModels()
	assert.Equal(t, provider.DefaultModels, allowed)
}

func TestManager_DefaultsApplication(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir)

	// Create minimal YAML config
	yamlConfig := `
providers:
  - name: "openrouter"
    api_key: "test-key"
  - name: "nonexistent"
    api_key: "test-key"
`

	yamlPath := filepath.Join(tempDir, DefaultYAMLFilename)
	err := os.WriteFile(yamlPath, []byte(yamlConfig), 0644)
	require.NoError(t, err)

	cfg, err := mgr.Load()
	require.NoError(t, err)

	// Basic defaults should be applied
	assert.Equal(t, DefaultHost, cfg.Host)
	assert.Equal(t, DefaultPort, cfg.Port)

	// Provider defaults should be applied
	openrouter := cfg.Providers[0]
	assert.Equal(t, DefaultProviderURLs["openrouter"], openrouter.APIBase)
	assert.Equal(t, DefaultProviderModels["openrouter"], openrouter.DefaultModels)

	// Nonexistent provider should not have URL or models
	nonexistent := cfg.Providers[1]
	assert.Empty(t, nonexistent.APIBase)
	assert.Empty(t, nonexistent.DefaultModels)
}

func TestManager_FileDetection(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir)

	// No files exist
	assert.False(t, mgr.Exists())
	assert.False(t, mgr.HasYAML())
	assert.False(t, mgr.HasJSON())

	// Create JSON file
	jsonPath := filepath.Join(tempDir, DefaultConfigFilename)
	err := os.WriteFile(jsonPath, []byte(`{"HOST": "127.0.0.1"}`), 0644)
	require.NoError(t, err)

	assert.True(t, mgr.Exists())
	assert.False(t, mgr.HasYAML())
	assert.True(t, mgr.HasJSON())
	assert.Equal(t, jsonPath, mgr.GetPath()) // Should return JSON path

	// Create YAML file (should take precedence)
	yamlPath := filepath.Join(tempDir, DefaultYAMLFilename)
	err = os.WriteFile(yamlPath, []byte(`host: "0.0.0.0"`), 0644)
	require.NoError(t, err)

	assert.True(t, mgr.Exists())
	assert.True(t, mgr.HasYAML())
	assert.True(t, mgr.HasJSON())
	assert.Equal(t, yamlPath, mgr.GetPath()) // Should return YAML path
}
