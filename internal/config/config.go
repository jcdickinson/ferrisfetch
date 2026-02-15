package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

type ApiKeyConfig struct {
	Value string `mapstructure:"-"`
	Path  string `mapstructure:"path"`
}

type VoyageAIConfig struct {
	ApiKey      ApiKeyConfig `mapstructure:"api_key"`
	Model       string       `mapstructure:"model"`
	RerankModel string       `mapstructure:"rerank_model"`
}

type DaemonConfig struct {
	ExpirationSeconds int `mapstructure:"expiration_seconds"`
}

type Config struct {
	VoyageAI VoyageAIConfig `mapstructure:"voyage_ai"`
	Daemon   DaemonConfig   `mapstructure:"daemon"`
}

// cacheBase returns the base cache directory for ferrisfetch.
// Checks XDG_CACHE_HOME, then ~/.cache, then /tmp/ferrisfetch as fallback.
func cacheBase() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "ferrisfetch")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "ferrisfetch")
	}
	return filepath.Join(os.TempDir(), "ferrisfetch")
}

// DBPath returns the path to the DuckDB database file.
func DBPath() string {
	return filepath.Join(cacheBase(), "db.db")
}

// CASDir returns the path to the content-addressable storage directory.
func CASDir() string {
	return filepath.Join(cacheBase(), "cas")
}

// JSONCacheDir returns the path to the JSON cache directory.
func JSONCacheDir() string {
	return filepath.Join(cacheBase(), "json")
}

// LogPath returns the path to the daemon's log file.
func LogPath() string {
	return filepath.Join(cacheBase(), "daemon.log")
}

// SocketPath returns the path to the daemon's unix socket.
func SocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "ferrisfetch", "daemon.sock")
	}
	return filepath.Join(fmt.Sprintf("/run/user/%d", os.Getuid()), "ferrisfetch", "daemon.sock")
}

func InitializeViper() error {
	viper.SetConfigName("config")
	viper.SetConfigType("toml")

	viper.AddConfigPath(".")
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		viper.AddConfigPath(filepath.Join(xdg, "ferrisfetch"))
	} else if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(home, ".config", "ferrisfetch"))
	}

	viper.SetDefault("voyage_ai.model", "voyage-3.5")
	viper.SetDefault("voyage_ai.rerank_model", "rerank-lite-1")
	viper.SetDefault("daemon.expiration_seconds", 600)

	viper.SetEnvPrefix("FERRISFETCH")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}
	return nil
}

func stringToApiKeyConfigHookFunc() mapstructure.DecodeHookFunc {
	return func(f, t reflect.Type, data interface{}) (interface{}, error) {
		if t != reflect.TypeOf(ApiKeyConfig{}) {
			return data, nil
		}
		if f.Kind() == reflect.String {
			return ApiKeyConfig{Value: data.(string)}, nil
		}
		return data, nil
	}
}

func Load() (*Config, error) {
	if err := InitializeViper(); err != nil {
		return nil, err
	}

	var config Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: stringToApiKeyConfigHookFunc(),
		Result:     &config,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(viper.AllSettings()); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := resolveApiKey(&config.VoyageAI.ApiKey); err != nil {
		return nil, fmt.Errorf("failed to resolve VoyageAI API key: %w", err)
	}

	return &config, nil
}

func resolveApiKey(apiKey *ApiKeyConfig) error {
	if envKey := viper.GetString("voyage_ai.api_key"); envKey != "" {
		if !strings.HasPrefix(envKey, "/") && !strings.HasPrefix(envKey, "./") && !strings.HasPrefix(envKey, "~/") {
			apiKey.Value = envKey
			return nil
		}
		apiKey.Path = envKey
	}

	if apiKey.Path != "" {
		if strings.HasPrefix(apiKey.Path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				apiKey.Path = filepath.Join(home, apiKey.Path[2:])
			}
		}
		keyBytes, err := os.ReadFile(apiKey.Path)
		if err != nil {
			return fmt.Errorf("failed to read API key from file %s: %w", apiKey.Path, err)
		}
		apiKey.Value = strings.TrimSpace(string(keyBytes))
	}

	return nil
}
