package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env   string
	Port  int
	LogLevel string

	DBHost string
	DBPort int
	DBName string
	DBUser string
	DBPass string

	JWTSecret string
	JWTTTL    time.Duration

	TURNSecret string
	TURNRealm  string
	TURNHost   string
	TURNPort   int

	PublicURL string
	MediaDir  string
	MaxUpload int64
	CORSOrigins []string
}

func envStr(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}

func envInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func envInt64(k string, def int64) int64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return i
}

func envDuration(k, def string) time.Duration {
	v := envStr(k, def)
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("config: invalid duration %q for %s, using %s", v, k, def)
		return 30 * 24 * time.Hour
	}
	return d
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Env:      envStr("QWAS_ENV", "development"),
		Port:     envInt("QWAS_PORT", 4000),
		LogLevel: envStr("QWAS_LOG_LEVEL", "info"),

		DBHost: envStr("QWAS_DB_HOST", "localhost"),
		DBPort: envInt("QWAS_DB_PORT", 5432),
		DBName: envStr("QWAS_DB_NAME", "qwas_app"),
		DBUser: envStr("QWAS_DB_USER", "qwas_app"),
		DBPass: envStr("QWAS_DB_PASS", ""),

		JWTSecret: envStr("QWAS_JWT_SECRET", ""),
		JWTTTL:    envDuration("QWAS_JWT_TTL", "720h"),

		TURNSecret: envStr("QWAS_TURN_SECRET", ""),
		TURNRealm:  envStr("QWAS_TURN_REALM", "api-qwas.academinctools.pw"),
		TURNHost:   envStr("QWAS_TURN_HOST", ""),
		TURNPort:   envInt("QWAS_TURN_PORT", 3478),

		PublicURL: envStr("QWAS_PUBLIC_URL", "http://localhost:4000"),
		MediaDir:  envStr("QWAS_MEDIA_DIR", "./media"),
		MaxUpload: int64(envInt("QWAS_MAX_UPLOAD_MB", 200)) * 1024 * 1024,

		CORSOrigins: strings.Split(envStr("QWAS_CORS_ORIGINS", "*"), ","),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("QWAS_JWT_SECRET is required")
	}
	if cfg.DBPass == "" {
		return nil, fmt.Errorf("QWAS_DB_PASS is required")
	}

	return cfg, nil
}
