package config

import (
	"os"
	"testing"
)

func TestConfig_Defaults(t *testing.T) {
	// Clear env vars
	os.Unsetenv("PORT")
	os.Unsetenv("DATA_DIR")
	os.Unsetenv("DAV_BACKUP_FOLDER")
	os.Unsetenv("DAV_USER")
	os.Unsetenv("DAV_PASS")
	os.Unsetenv("READ_ONLY")
	os.Unsetenv("R2_ENDPOINT")
	os.Unsetenv("R2_ACCESS_KEY")
	os.Unsetenv("R2_SECRET_KEY")
	os.Unsetenv("R2_BUCKET")
	os.Unsetenv("DISCORD_WEBHOOK_URL")

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

	if cfg.Port != ":8080" {
		t.Errorf("expected default Port :8080, got %s", cfg.Port)
	}
	if cfg.BackupFolder != "vbook_backup" {
		t.Errorf("expected default BackupFolder vbook_backup, got %s", cfg.BackupFolder)
	}
	if cfg.HasR2() {
		t.Errorf("expected HasR2 to be false by default")
	}
	if cfg.HasDiscord() {
		t.Errorf("expected HasDiscord to be false by default")
	}
}

func TestConfig_IntrospectionHelpers(t *testing.T) {
	cfg := &Config{
		R2Endpoint:        "https://account.r2.cloudflarestorage.com",
		R2AccessKey:       "access123",
		R2SecretKey:       "secret456",
		R2Bucket:          "vbook-backup",
		DiscordWebhookURL: "https://discord.com/api/webhooks/123/xyz",
	}

	if !cfg.HasR2() {
		t.Errorf("expected HasR2 to be true when all R2 fields are set")
	}
	if !cfg.HasDiscord() {
		t.Errorf("expected HasDiscord to be true when Discord URL is set")
	}

	// Partial R2 config should return false
	cfg.R2SecretKey = ""
	if cfg.HasR2() {
		t.Errorf("expected HasR2 to be false when R2SecretKey is missing")
	}

	cfg.R2SecretKey = "secret456"
	cfg.R2Bucket = ""
	if cfg.HasR2() {
		t.Errorf("expected HasR2 to be false when R2Bucket is missing")
	}

	cfg.R2Bucket = "vbook-backup"
	cfg.R2Endpoint = ""
	if cfg.HasR2() {
		t.Errorf("expected HasR2 to be false when R2Endpoint is missing")
	}

	cfg.R2Endpoint = "https://account.r2.cloudflarestorage.com"
	cfg.R2AccessKey = ""
	if cfg.HasR2() {
		t.Errorf("expected HasR2 to be false when R2AccessKey is missing")
	}
}

func TestConfig_LoadDotEnvFile(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test_env_*.env")
	if err != nil {
		t.Fatalf("failed to create temp env file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	content := `# Test comment
TEST_KEY_1=value1
TEST_KEY_QUOTED="quoted_value"
TEST_KEY_SINGLE='single_value'
# ANOTHER_COMMENT=ignored
`
	if _, err := tempFile.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write temp env file: %v", err)
	}
	tempFile.Close()

	os.Unsetenv("TEST_KEY_1")
	os.Unsetenv("TEST_KEY_QUOTED")
	os.Unsetenv("TEST_KEY_SINGLE")

	loadDotEnvFile(tempFile.Name())

	if os.Getenv("TEST_KEY_1") != "value1" {
		t.Errorf("expected TEST_KEY_1 to be 'value1', got '%s'", os.Getenv("TEST_KEY_1"))
	}
	if os.Getenv("TEST_KEY_QUOTED") != "quoted_value" {
		t.Errorf("expected TEST_KEY_QUOTED to be 'quoted_value', got '%s'", os.Getenv("TEST_KEY_QUOTED"))
	}
	if os.Getenv("TEST_KEY_SINGLE") != "single_value" {
		t.Errorf("expected TEST_KEY_SINGLE to be 'single_value', got '%s'", os.Getenv("TEST_KEY_SINGLE"))
	}
}
