//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWriteTemplatesConfigFileHonorsUmaskForNewFile(t *testing.T) {
	configFilePath := filepath.Join(t.TempDir(), TemplateConfigFileName)
	previousUmask := syscall.Umask(0o777)
	t.Cleanup(func() { syscall.Umask(previousUmask) })

	if err := writeTemplatesConfigFile(configFilePath, []byte("current"), os.Rename); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(configFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0 {
		t.Fatalf("expected mode 0000 under restrictive umask, got %04o", got)
	}
}

func TestWriteTemplatesConfigFileRejectsFIFOThroughSymlink(t *testing.T) {
	parent := t.TempDir()
	targetPath := filepath.Join(parent, "managed-config")
	if err := syscall.Mkfifo(targetPath, 0o600); err != nil {
		t.Fatal(err)
	}
	configFilePath := filepath.Join(parent, TemplateConfigFileName)
	if err := os.Symlink(targetPath, configFilePath); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	if err := writeTemplatesConfigFile(configFilePath, []byte("current"), os.Rename); err == nil {
		t.Fatal("expected non-regular target rejection")
	}
	info, err := os.Lstat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("expected FIFO target to remain, got mode %v", info.Mode())
	}
	info, err = os.Lstat(configFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected config symlink to remain")
	}
}
