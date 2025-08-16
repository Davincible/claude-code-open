package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/Davincible/claude-code-open/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `Manage the LLM proxy router configuration.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration interactively",
	Long:  `Initialize configuration by prompting for provider details.`,
	RunE:  runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current configuration.`,
	RunE:  runConfigShow,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration",
	Long:  `Validate the current configuration for errors.`,
	RunE:  runConfigValidate,
}

var configGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate example YAML configuration",
	Long:  `Generate an example YAML configuration file with all available providers.`,
	RunE:  runConfigGenerate,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configValidateCmd)
	configCmd.AddCommand(configGenerateCmd)

	// Add flags for generate command
	configGenerateCmd.Flags().BoolP("force", "f", false, "Overwrite existing configuration file")
}

func runConfigInit(cmd *cobra.Command, _ []string) error {
	color.Blue("Claude Code Router Configuration Setup")
	color.Yellow("Follow the prompts to configure your LLM providers.")

	reader := bufio.NewReader(os.Stdin)

	// Get provider details
	fmt.Print("\nProvider Name (e.g., openrouter, openai): ")

	providerName, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading provider name: %w", err)
	}

	providerName = strings.TrimSpace(providerName)

	fmt.Print("API Key: ")

	apiKey, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading API key: %w", err)
	}

	apiKey = strings.TrimSpace(apiKey)

	fmt.Print("API Base URL: ")

	baseURL, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading base URL: %w", err)
	}

	baseURL = strings.TrimSpace(baseURL)

	fmt.Print("Default Model: ")

	model, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading model: %w", err)
	}

	model = strings.TrimSpace(model)

	// Optional router API key
	fmt.Print("Router API Key (optional, for authentication): ")

	routerAPIKey, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading router API key: %w", err)
	}
	routerAPIKey = strings.TrimSpace(routerAPIKey)

	// Create configuration
	cfg := &config.Config{
		Host:   config.DefaultHost,
		Port:   config.DefaultPort,
		APIKey: routerAPIKey,
		Providers: []config.Provider{
			{
				Name:    providerName,
				APIBase: baseURL,
				APIKey:  apiKey,
				Models:  []string{model},
			},
		},
		Router: config.RouterConfig{
			Default: fmt.Sprintf("%s,%s", providerName, model),
		},
	}

	// Save configuration
	if err := cfgMgr.Save(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	color.Green("Configuration saved successfully to: %s", cfgMgr.GetPath())
	color.Cyan("You can now start the router with: cco start")

	return nil
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
    if !cfgMgr.Exists() {
        color.Yellow("No configuration found. Run 'cco config init' or 'cco config generate' to create one.")
        return nil
    }

    cfg, err := cfgMgr.Load()
    if err != nil {
        return fmt.Errorf("failed to load configuration: %w", err)
    }

    color.Blue("Current Configuration:")
    fmt.Printf("  %-15s: %s\n", "Host", cfg.Host)
    fmt.Printf("  %-15s: %d\n", "Port", cfg.Port)
    fmt.Printf("  %-15s: %s\n", "API Key", maskString(cfg.APIKey))
    fmt.Printf("  %-15s: %s\n", "Config Path", cfgMgr.GetPath())

    // Show config file type
    configType := "JSON"
    if cfgMgr.HasYAML() {
        configType = "YAML"
    }
    fmt.Printf("  %-15s: %s\n", "Format", configType)

    // Show domain mappings if present
    if len(cfg.DomainMappings) > 0 {
        fmt.Println("\nDomain Mappings:")
        for domain, provider := range cfg.DomainMappings {
            fmt.Printf("  %-15s → %s\n", domain, provider)
        }
    }

    fmt.Println("\nProviders:")
    for _, provider := range cfg.Providers {
        fmt.Printf("  - Name: %s\n", provider.Name)
        fmt.Printf("    URL: %s\n", provider.APIBase)
        fmt.Printf("    API Key: %s\n", maskString(provider.APIKey))

        if len(provider.DefaultModels) > 0 {
            fmt.Printf("    Default Models: %v\n", provider.DefaultModels)
        }

        if len(provider.ModelWhitelist) > 0 {
            fmt.Printf("    Model Whitelist: %v\n", provider.ModelWhitelist)
        }

        if len(provider.Models) > 0 {
            fmt.Printf("    Models: %v\n", provider.Models)
        }

        fmt.Println()
    }

    fmt.Println("Router Configuration:")
    fmt.Printf("  %-15s: %s\n", "Default", cfg.Router.Default)

    if cfg.Router.Think != "" {
        fmt.Printf("  %-15s: %s\n", "Think", cfg.Router.Think)
    }

    if cfg.Router.Background != "" {
        fmt.Printf("  %-15s: %s\n", "Background", cfg.Router.Background)
    }

    if cfg.Router.LongContext != "" {
        fmt.Printf("  %-15s: %s\n", "Long Context", cfg.Router.LongContext)
    }

    if cfg.Router.WebSearch != "" {
        fmt.Printf("  %-15s: %s\n", "Web Search", cfg.Router.WebSearch)
    }

    return nil
}


func runConfigValidate(cmd *cobra.Command, _ []string) error {
    if !cfgMgr.Exists() {
        return errors.New("no configuration found")
    }

    cfg, err := cfgMgr.Load()
    if err != nil {
        return fmt.Errorf("failed to load configuration: %w", err)
    }

    // Validation logic
    var validationErrors []string

    if len(cfg.Providers) == 0 {
        validationErrors = append(validationErrors, "no providers configured")
    }

    // Validate providers
    providerNames := make(map[string]bool)
    for i, provider := range cfg.Providers {
        if provider.Name == "" {
            validationErrors = append(validationErrors, fmt.Sprintf("provider %d: name is required", i))
        } else {
            providerNames[provider.Name] = true
        }

        if provider.APIBase == "" {
            validationErrors = append(validationErrors, fmt.Sprintf("provider %d: API base URL is required", i))
        }

        if provider.APIKey == "" {
            validationErrors = append(validationErrors, fmt.Sprintf("provider %d: API key is required", i))
        }
    }

    // Validate domain mappings reference valid providers
    if len(cfg.DomainMappings) > 0 {
        for domain, providerName := range cfg.DomainMappings {
            if domain == "" {
                validationErrors = append(validationErrors, "domain mapping: empty domain not allowed")
                continue
            }
            
            // Check if referenced provider exists (either in config or built-in)
            builtInProviders := map[string]bool{
                "openrouter": true,
                "openai":     true,
                "anthropic":  true,
                "nvidia":     true,
                "gemini":     true,
            }
            
            if !providerNames[providerName] && !builtInProviders[providerName] {
                validationErrors = append(validationErrors, 
                    fmt.Sprintf("domain mapping '%s → %s': provider '%s' not found", 
                        domain, providerName, providerName))
            }
        }
    }

    if cfg.Router.Default == "" {
        validationErrors = append(validationErrors, "default router model is required")
    }

    if len(validationErrors) > 0 {
        color.Red("Configuration validation failed:")
        for _, err := range validationErrors {
            fmt.Printf("  - %s\n", err)
        }
        return errors.New("configuration validation failed")
    }

    color.Green("Configuration is valid!")
    
    // Show additional info about domain mappings if present
    if len(cfg.DomainMappings) > 0 {
        color.Cyan("\nDomain mappings configured:")
        for domain, provider := range cfg.DomainMappings {
            fmt.Printf("  %s → %s\n", domain, provider)
        }
    }

    return nil
}


func runConfigGenerate(cmd *cobra.Command, _ []string) error {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	// Check if config already exists
	if cfgMgr.Exists() && !force {
		configType := "JSON"
		if cfgMgr.HasYAML() {
			configType = "YAML"
		}

		color.Yellow("Configuration file already exists (%s format): %s", configType, cfgMgr.GetPath())
		color.Cyan("Use --force to overwrite, or 'cco config show' to view current config")

		return nil
	}

    // Generate example YAML config
    if err := cfgMgr.CreateExampleYAML(); err != nil {
        return fmt.Errorf("failed to create example configuration: %w", err)
    }

    color.Green("Example YAML configuration created: %s", cfgMgr.GetYAMLPath())
    color.Cyan("\nNext steps:")
    fmt.Println("1. Edit the configuration file to add your API keys")
    fmt.Println("2. Customize provider settings and model whitelists as needed")
    fmt.Println("3. Configure domain_mappings to route local servers to providers")
    fmt.Println("4. Run 'cco config validate' to check your configuration")
    fmt.Println("5. Start the router with 'cco start'")

    color.Yellow("\nNote: The configuration includes:")
    fmt.Println("- All 5 supported providers (OpenRouter, OpenAI, Anthropic, Nvidia, Gemini)")
    fmt.Println("- Domain mappings for local model support (localhost, 127.0.0.1, etc.)")
    fmt.Println("- Example local LM Studio provider configuration")
    fmt.Println("- Router configuration for different use cases")

    return nil
}

func maskString(s string) string {
	if s == "" {
		return "(not set)"
	}

	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}

	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}
