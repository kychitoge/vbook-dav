package config

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config contains all server configuration parameters for vbook-dav.
type Config struct {
	Port         string
	DataDir      string
	BackupFolder string
	Username     string
	Password     string
	ReadOnly     bool

	// Cloudflare R2 replication settings (optional — empty means local-only mode)
	R2Endpoint  string
	R2AccessKey string
	R2SecretKey string
	R2Bucket    string

	// Discord notification settings (optional — empty means notifications disabled)
	DiscordWebhookURL string
}

// Load reads configuration from environment variables and CLI flags (flags take precedence).
func Load() *Config {
	loadDotEnv()

	cfg := &Config{
		Port:              getEnv("PORT", ":8080"),
		DataDir:           getEnv("DATA_DIR", "./data"),
		BackupFolder:      getEnv("DAV_BACKUP_FOLDER", "vbook_backup"),
		Username:          getEnv("DAV_USER", "vbook"),
		Password:          getEnv("DAV_PASS", "vbook123"),
		ReadOnly:          getEnvBool("READ_ONLY", false),
		R2Endpoint:        getEnv("R2_ENDPOINT", ""),
		R2AccessKey:       getEnv("R2_ACCESS_KEY", ""),
		R2SecretKey:       getEnv("R2_SECRET_KEY", ""),
		R2Bucket:          getEnv("R2_BUCKET", ""),
		DiscordWebhookURL: getEnv("DISCORD_WEBHOOK_URL", ""),
	}

	// Normalize port format
	if cfg.Port != "" && !strings.HasPrefix(cfg.Port, ":") {
		cfg.Port = ":" + cfg.Port
	}

	// Register CLI flags
	portFlag := flag.String("port", cfg.Port, "WebDAV listening port (for example, :8080)")
	dataDirFlag := flag.String("data-dir", cfg.DataDir, "Directory to store WebDAV files")
	backupFolderFlag := flag.String("backup-folder", cfg.BackupFolder, "Subdirectory name for vBook backups")
	userFlag := flag.String("user", cfg.Username, "WebDAV basic auth username")
	passFlag := flag.String("pass", cfg.Password, "WebDAV basic auth password")
	readOnlyFlag := flag.Bool("read-only", cfg.ReadOnly, "Enable read-only mode")

	r2EndpointFlag := flag.String("r2-endpoint", cfg.R2Endpoint, "Cloudflare R2 S3 API endpoint URL")
	r2AccessKeyFlag := flag.String("r2-access-key", cfg.R2AccessKey, "Cloudflare R2 access key ID")
	r2SecretKeyFlag := flag.String("r2-secret-key", cfg.R2SecretKey, "Cloudflare R2 secret access key")
	r2BucketFlag := flag.String("r2-bucket", cfg.R2Bucket, "Cloudflare R2 bucket name")
	discordWebhookFlag := flag.String("discord-webhook", cfg.DiscordWebhookURL, "Discord incoming webhook URL for notifications")

	flag.Parse()

	cfg.Port = *portFlag
	if !strings.HasPrefix(cfg.Port, ":") {
		cfg.Port = ":" + cfg.Port
	}
	cfg.DataDir = filepath.Clean(*dataDirFlag)
	cfg.BackupFolder = strings.Trim(filepath.Clean(*backupFolderFlag), "/\\")
	if cfg.BackupFolder == "" || cfg.BackupFolder == "." {
		cfg.BackupFolder = "vbook_backup"
	}
	cfg.Username = strings.TrimSpace(*userFlag)
	cfg.Password = *passFlag
	cfg.ReadOnly = *readOnlyFlag

	// Normalize R2 endpoint by stripping trailing slashes
	cfg.R2Endpoint = strings.TrimRight(strings.TrimSpace(*r2EndpointFlag), "/")
	cfg.R2AccessKey = strings.TrimSpace(*r2AccessKeyFlag)
	cfg.R2SecretKey = strings.TrimSpace(*r2SecretKeyFlag)
	cfg.R2Bucket = strings.TrimSpace(*r2BucketFlag)
	cfg.DiscordWebhookURL = strings.TrimSpace(*discordWebhookFlag)

	return cfg
}

// HasR2 reports whether Cloudflare R2 replication credentials are fully configured.
func (c *Config) HasR2() bool {
	return c.R2Endpoint != "" && c.R2Bucket != "" && c.R2AccessKey != "" && c.R2SecretKey != ""
}

// HasDiscord reports whether Discord notification webhook is configured.
func (c *Config) HasDiscord() bool {
	return c.DiscordWebhookURL != ""
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		b, err := strconv.ParseBool(strings.TrimSpace(val))
		if err == nil {
			return b
		}
	}
	return defaultVal
}

// loadDotEnv parses a .env file in the current working directory if it exists,
// populating os environment variables that are not yet set.
func loadDotEnv() {
	loadDotEnvFile(".env")
}

func loadDotEnvFile(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
