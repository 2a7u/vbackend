package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoCommittedSecretsScansTrackedFiles ensures no production secrets or raw keys are leaked into source files.
func TestNoCommittedSecretsScansTrackedFiles(t *testing.T) {
	root := "../.."
	var scannedFiles int

	// Disallowed substrings that indicate leaked production credentials
	forbiddenPatterns := []string{
		"VAULT_HMAC_SECRET=prod_",
		"VAULT_ENCRYPTION_KEY=prod_",
		"AWS_SECRET_ACCESS_KEY",
		"PRIVATE KEY-----",
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".github" || name == "vendor" || name == "build" || name == "dist" || name == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".sql" && ext != ".yaml" && ext != ".yml" && ext != ".json" && ext != ".sh" {
			return nil
		}

		// Skip tests or examples that legitimately define test/mock secrets
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "test") || strings.HasSuffix(path, ".example") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		scannedFiles++
		contentStr := string(content)

		for _, pat := range forbiddenPatterns {
			if strings.Contains(contentStr, pat) {
				t.Errorf("SECURITY LEAK DETECTED: File %s contains forbidden secret pattern %q", path, pat)
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("WalkDir failed: %v", err)
	}

	if scannedFiles == 0 {
		t.Errorf("Expected to scan files, but none were scanned")
	}
}

// TestVaultManagerDoesNotLeakInStringFormat ensures fmt.Sprint on VaultManager does not dump raw keys.
func TestVaultManagerDoesNotLeakInStringFormat(t *testing.T) {
	vm := NewVaultManagerFromEnv()
	rawRepresentation := fmt.Sprintf("%v %+v %#v", vm, vm, vm)

	// Verify that raw 32-byte key contents are not exposed in plaintext format
	if strings.Contains(rawRepresentation, "VENUS_VAULT_AES256_ROOT_KEY_2026_PRODUCTION") {
		t.Errorf("VaultManager representation exposed raw derivation key string!")
	}
}
