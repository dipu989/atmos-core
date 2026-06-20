package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App       AppConfig
	DB        DBConfig
	JWT       JWTConfig
	Google    GoogleOAuthConfig
	Email     EmailConfig
	Push      PushConfig
	ClaudeBin string // CLAUDE_BIN — path to claude CLI binary (default: "claude")
}

type PushConfig struct {
	FCMServiceAccountJSON string // FCM_SERVICE_ACCOUNT_JSON — path to Firebase service account key file
}

type EmailConfig struct {
	Region   string // AWS_REGION — e.g. "ap-south-1"
	FromAddr string // SES_FROM_ADDRESS — e.g. "Atmos <noreply@atmosapp.dev>"
}

type AppConfig struct {
	Env             string
	Port            string
	FrontendURL     string // APP_FRONTEND_URL — where to redirect after OAuth (e.g. https://atmosapp.dev)
	CORSAllowOrigin string // CORS_ALLOW_ORIGIN — required in production
	InternalSyncKey string // INTERNAL_SYNC_KEY — shared secret for /internal/* endpoints
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxOpen  int
	MaxIdle  int
	MaxLife  time.Duration
}

type JWTConfig struct {
	AccessSecret    string
	RefreshSecret   string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type GoogleOAuthConfig struct {
	ClientID         string
	IosClientID      string // iOS native Sign-In client ID (separate audience from web client)
	ClientSecret     string
	RedirectURL      string // For web Sign-In callback
	GmailRedirectURL string // GOOGLE_GMAIL_REDIRECT_URL — for Gmail connect callback
	MapsAPIKey       string // GOOGLE_MAPS_API_KEY — optional; enables address geocoding
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	env := getEnv("APP_ENV", "development")

	corsOrigin := getEnv("CORS_ALLOW_ORIGIN", "")
	if corsOrigin == "" {
		if env == "production" {
			return nil, fmt.Errorf("required environment variable %q is not set", "CORS_ALLOW_ORIGIN")
		}
		corsOrigin = "http://localhost:3000"
	}

	cfg := &Config{
		App: AppConfig{
			Env:             env,
			Port:            getEnv("APP_PORT", "8080"),
			FrontendURL:     getEnv("APP_FRONTEND_URL", "http://localhost:3000"),
			CORSAllowOrigin: corsOrigin,
			InternalSyncKey: getEnv("INTERNAL_SYNC_KEY", ""),
		},
		DB: DBConfig{
			Host:     mustEnv("DB_HOST"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     mustEnv("DB_USER"),
			Password: mustEnv("DB_PASSWORD"),
			Name:     mustEnv("DB_NAME"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			MaxOpen:  getEnvInt("DB_MAX_OPEN", 25),
			MaxIdle:  getEnvInt("DB_MAX_IDLE", 10),
			MaxLife:  getEnvDuration("DB_MAX_LIFE", 5*time.Minute),
		},
		JWT: JWTConfig{
			AccessSecret:    mustEnv("JWT_ACCESS_SECRET"),
			RefreshSecret:   mustEnv("JWT_REFRESH_SECRET"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 30*24*time.Hour),
		},
		Google: GoogleOAuthConfig{
			ClientID:         getEnv("GOOGLE_CLIENT_ID", ""),
			IosClientID:      getEnv("GOOGLE_IOS_CLIENT_ID", ""),
			ClientSecret:     getEnv("GOOGLE_CLIENT_SECRET", ""),
			RedirectURL:      getEnv("GOOGLE_REDIRECT_URL", ""),
			GmailRedirectURL: getEnv("GOOGLE_GMAIL_REDIRECT_URL", ""),
			MapsAPIKey:       getEnv("GOOGLE_MAPS_API_KEY", ""),
		},
		Email: EmailConfig{
			Region:   getEnv("AWS_REGION", "ap-south-1"),
			FromAddr: getEnv("SES_FROM_ADDRESS", "Atmos <noreply@atmosapp.dev>"),
		},
		Push: PushConfig{
			FCMServiceAccountJSON: getEnv("FCM_SERVICE_ACCOUNT_JSON", ""),
		},
		ClaudeBin: getEnv("CLAUDE_BIN", "claude"),
	}

	return cfg, nil
}

func (d *DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required environment variable %q is not set", key))
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
