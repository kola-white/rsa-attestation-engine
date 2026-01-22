package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Addr string
	Spaces SpacesConfig

	Kratos KratosConfig
	Auth   AuthConfig
	// in type Config struct:
	JWTSecret   string
	JWTIssuer   string
	JWTAudience string
}

type KratosConfig struct {
	PublicBaseURL string // e.g. https://auth.cvera.app
}

type AuthConfig struct {
	JWTSecret string // HS256 secret (long random)
	Issuer    string // e.g. cvera-api
	Audience  string // e.g. cvera-app
	DBDSN	 string 
	RefreshTokenHMACKey string 
}

type SpacesConfig struct {
	AccessKey string
	SecretKey string
	Region    string
	Endpoint  string
	Bucket    string
}

// Load reads environment variables and returns a validated config.
// - EVT_API_ADDR optional (defaults to 127.0.0.1:8080)
// - DO_* are read for Spaces credentials/region/endpoint
// - EVT_S3_BUCKET required once you start calling S3
func Load() (Config, error) {
	cfg := Config{
		Addr: getenv("EVT_API_ADDR", "127.0.0.1:8080"),
		Spaces: SpacesConfig{
			AccessKey: os.Getenv("DO_SPACES_KEY"),
			SecretKey: os.Getenv("DO_SPACES_SECRET"),
			Region:    getenv("DO_REGION", "sfo3"),
			Endpoint:  os.Getenv("DO_ENDPOINT"),
			Bucket:    os.Getenv("EVT_S3_BUCKET"),
		},
		Kratos: KratosConfig{
		PublicBaseURL: getenv("KRATOS_PUBLIC_BASE_URL", "https://auth.cvera.app"),
	},
		Auth: AuthConfig{
		JWTSecret: os.Getenv("EVT_JWT_SECRET"),
		Issuer:    getenv("EVT_JWT_ISS", "cvera-api"),
		Audience:  getenv("EVT_JWT_AUD", "cvera-app"),
		DBDSN:     os.Getenv("EVT_DB_DSN"),
		RefreshTokenHMACKey: os.Getenv("EVT_REFRESH_TOKEN_HMAC_KEY"),
	},
	}

	// Normalize endpoint slightly (optional but helps prevent silly mistakes)
	cfg.Spaces.Endpoint = strings.TrimSpace(cfg.Spaces.Endpoint)
	cfg.Spaces.Endpoint = strings.TrimRight(cfg.Spaces.Endpoint, "/")

	// Validation: only enforce Spaces vars if you actually plan to hit S3.
	// For now, you can keep this strict or relaxed. I’m making it strict
	// because your next endpoint (/commit) depends on HEAD.
	var errs []error
	if cfg.Spaces.AccessKey == "" {
		errs = append(errs, errors.New("missing DO_SPACES_KEY"))
	}
	if cfg.Spaces.SecretKey == "" {
		errs = append(errs, errors.New("missing DO_SPACES_SECRET"))
	}
	if cfg.Spaces.Endpoint == "" {
		errs = append(errs, errors.New("missing DO_ENDPOINT (e.g. https://sfo3.digitaloceanspaces.com)"))
	}
	if cfg.Spaces.Bucket == "" {
		errs = append(errs, errors.New("missing EVT_S3_BUCKET (certis-evidence-uploads)"))
	}

	if cfg.Auth.JWTSecret == "" {
			errs = append(errs, errors.New("missing EVT_JWT_SECRET"))
	}
	if cfg.Auth.DBDSN == "" {
			errs = append(errs, errors.New("missing EVT_DB_DSN"))
	}
	if cfg.Auth.RefreshTokenHMACKey == "" {
			errs = append(errs, errors.New("missing EVT_REFRESH_TOKEN_HMAC_KEY"))
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid config: %v", joinErrors(errs))
	}

	return cfg, nil
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func joinErrors(errs []error) string {
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e.Error())
	}
	return b.String()
}
