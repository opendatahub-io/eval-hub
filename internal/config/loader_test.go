package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eval-hub/eval-hub/internal/config"
	"github.com/eval-hub/eval-hub/internal/logging"
)

func TestLoadConfig(t *testing.T) {
	logger := logging.FallbackLogger()

	t.Run("loading config from tests directory", func(t *testing.T) {
		serviceConfig, err := config.LoadConfig(logger, "0.0.1", "local", time.Now().Format(time.RFC3339), "../../tests")
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		if serviceConfig == nil {
			t.Fatalf("Service config is nil")
		}
	})

	t.Run("setting environment variables", func(t *testing.T) {
		os.Setenv("MLFLOW_TRACKING_URI", "http://localhost:9999")
		t.Cleanup(func() {
			os.Unsetenv("MLFLOW_TRACKING_URI")
		})
		serviceConfig, err := config.LoadConfig(logger, "0.0.1", "local", time.Now().Format(time.RFC3339), "../../tests")
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		if serviceConfig == nil {
			t.Fatalf("Service config is nil")
		}
		if serviceConfig.MLFlow.TrackingURI != "http://localhost:9999" {
			t.Fatalf("MLFlow tracking URI is not http://localhost:9999, got %s", serviceConfig.MLFlow.TrackingURI)
		}
	})

	t.Run("loading config from CONFIG_PATH", func(t *testing.T) {
		// Create a temp directory with a config.yaml that has a custom port
		tmpDir := t.TempDir()
		configContent := `
service:
  port: 9999
  ready_file: "/tmp/repo-ready"
  termination_file: "/tmp/termination-log"
database:
  driver: sqlite
  url: "file::memory:?mode=memory&cache=shared"
`
		err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0600)
		if err != nil {
			t.Fatalf("Failed to write temp config: %v", err)
		}

		configPath := filepath.Join(tmpDir, "config.yaml")
		os.Setenv("CONFIG_PATH", configPath)
		t.Cleanup(func() {
			os.Unsetenv("CONFIG_PATH")
		})

		// Pass no explicit dirs (LoadConfig should pick up CONFIG_PATH)
		serviceConfig, err := config.LoadConfig(logger, "0.0.1", "local", time.Now().Format(time.RFC3339))
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		if serviceConfig == nil {
			t.Fatalf("Service config is nil")
		}
		if serviceConfig.Service.Port != 9999 {
			t.Fatalf("Expected port 9999 from CONFIG_PATH config, got %d", serviceConfig.Service.Port)
		}
	})

	t.Run("CONFIG_PATH directory takes precedence over defaults", func(t *testing.T) {
		// Create a temp directory with a config that has a distinct port
		tmpDir := t.TempDir()
		configContent := `
service:
  port: 7777
  ready_file: "/tmp/repo-ready"
  termination_file: "/tmp/termination-log"
database:
  driver: sqlite
  url: "file::memory:?mode=memory&cache=shared"
`
		err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0600)
		if err != nil {
			t.Fatalf("Failed to write temp config: %v", err)
		}

		configPath := filepath.Join(tmpDir, "config.yaml")
		os.Setenv("CONFIG_PATH", configPath)
		t.Cleanup(func() {
			os.Unsetenv("CONFIG_PATH")
		})

		// Also pass the tests directory (CONFIG_PATH dir should be searched first)
		serviceConfig, err := config.LoadConfig(logger, "0.0.1", "local", time.Now().Format(time.RFC3339), "../../tests")
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		if serviceConfig.Service.Port != 7777 {
			t.Fatalf("Expected CONFIG_PATH config (port 7777) to take precedence, got %d", serviceConfig.Service.Port)
		}
	})

	t.Run("loading config from secrets directory", func(t *testing.T) {
		// create a secret and store in /tmp/db_password
		secret := "mysecret"
		secretPath := "/tmp/db_password"
		err := os.WriteFile(secretPath, []byte(secret), 0600)
		if err != nil {
			t.Fatalf("Failed to create secret: %v", err)
		}
		t.Cleanup(func() {
			os.Remove(secretPath)
		})
		serviceConfig, err := config.LoadConfig(logger, "0.0.1", "local", time.Now().Format(time.RFC3339), "../../tests")
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		if serviceConfig == nil {
			t.Fatalf("Service config is nil")
		}
		if serviceConfig.Database == nil {
			t.Fatalf("Database config is nil")
		}
		db := *serviceConfig.Database
		if password, ok := db["password"]; ok {
			if password.(string) != secret {
				t.Fatalf("Database password is not %s, got %s", secret, password.(string))
			}
		} else {
			t.Fatalf("Database password is not set")
		}
	})
}
