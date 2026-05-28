package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerAddr string
	LogLevel   string

	// APIKey, if non-empty, requires every /api/* request to send a matching
	// X-API-Key header. Empty means public mode (default, local-dev friendly).
	APIKey string

	// ResearchTimeoutSeconds bounds the whole /api/research pipeline (planner
	// + DAG + writer). 0 disables it but is strongly discouraged in production
	// because a hung LLM call can otherwise leak goroutines forever.
	ResearchTimeoutSeconds int

	LLM      LLMConfig
	Postgres PostgresConfig
	Redis    RedisConfig

	TavilyAPIKey      string
	TavilySearchDepth string

	Agent AgentConfig
}

// AgentConfig tunes Phase 4 researcher ReAct + critic behaviour.
type AgentConfig struct {
	MaxSearchRounds    int
	MaxFollowUpQueries int
	CriticEnabled      bool
	CriticMinScore     int
	MaxCriticRetries   int
}

type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DB       string
}

func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		p.Host, p.Port, p.User, p.Password, p.DB,
	)
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// Load reads configuration from environment variables, optionally bootstrapped
// from a .env file in the working directory. Missing .env is not an error so
// production deployments can rely solely on real env vars.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		ServerAddr:             getEnv("SERVER_ADDR", ":8080"),
		LogLevel:               strings.ToLower(getEnv("LOG_LEVEL", "info")),
		APIKey:                 getEnv("API_KEY", ""),
		ResearchTimeoutSeconds: getEnvInt("RESEARCH_TIMEOUT_SECONDS", 180),
		LLM: LLMConfig{
			BaseURL: getEnv("LLM_BASE_URL", "https://api.deepseek.com/v1"),
			APIKey:  getEnv("LLM_API_KEY", ""),
			Model:   getEnv("LLM_MODEL", "deepseek-chat"),
		},
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "127.0.0.1"),
			Port:     getEnvInt("POSTGRES_PORT", 5432),
			User:     getEnv("POSTGRES_USER", "research"),
			Password: getEnv("POSTGRES_PASSWORD", "research"),
			DB:       getEnv("POSTGRES_DB", "research"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		TavilyAPIKey:      getEnv("TAVILY_API_KEY", ""),
		TavilySearchDepth: getEnv("TAVILY_SEARCH_DEPTH", "basic"),
		Agent: AgentConfig{
			MaxSearchRounds:    getEnvInt("RESEARCH_MAX_SEARCH_ROUNDS", 2),
			MaxFollowUpQueries: getEnvInt("RESEARCH_MAX_FOLLOWUP_QUERIES", 2),
			CriticEnabled:      getEnvBool("CRITIC_ENABLED", true),
			CriticMinScore:     getEnvInt("CRITIC_MIN_SCORE", 6),
			MaxCriticRetries:   getEnvInt("CRITIC_MAX_RETRIES", 1),
		},
	}

	if cfg.LLM.APIKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY is required (set it in .env)")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
