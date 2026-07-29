package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weekline.env")
	if err := os.WriteFile(path, []byte("# comment\nWEEKLINE_TEST_ONE=value\nWEEKLINE_TEST_TWO=\"quoted value\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEEKLINE_TEST_ONE", "existing")
	if err := loadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if value := os.Getenv("WEEKLINE_TEST_ONE"); value != "existing" {
		t.Fatalf("expected existing value to win, got %q", value)
	}
	if value := os.Getenv("WEEKLINE_TEST_TWO"); value != "quoted value" {
		t.Fatalf("expected quotes to be removed, got %q", value)
	}
}

func TestLoadEnvFileAcceptsUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weekline.env")
	const key = "WEEKLINE_TEST_UTF8_BOM_FIRST_KEY"
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte(key+"=recognized\n")...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	previous, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, previous)
		} else {
			_ = os.Unsetenv(key)
		}
	})
	if err := loadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if value := os.Getenv(key); value != "recognized" {
		t.Fatalf("expected BOM-prefixed first key to be recognized, got %q", value)
	}
}
