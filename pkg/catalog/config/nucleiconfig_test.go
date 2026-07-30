package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTemplatesConfigFilePreservesPreviousConfigOnReplaceFailure(t *testing.T) {
	configFilePath := filepath.Join(t.TempDir(), TemplateConfigFileName)
	previousContents := []byte(`{"nuclei-templates-version":"v1.0.0"}`)
	if err := os.WriteFile(configFilePath, previousContents, 0o600); err != nil {
		t.Fatal(err)
	}
	replaceErr := errors.New("replace failed")

	err := writeTemplatesConfigFile(configFilePath, []byte(`{"nuclei-templates-version":"v2.0.0"}`), func(string, string) error {
		return replaceErr
	})
	if !errors.Is(err, replaceErr) {
		t.Fatalf("expected replacement error, got %v", err)
	}
	contents, err := os.ReadFile(configFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != string(previousContents) {
		t.Fatalf("expected prior config contents %q, got %q", previousContents, contents)
	}
	temporaryFiles, err := filepath.Glob(configFilePath + ".tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("expected temporary config cleanup, found %v", temporaryFiles)
	}
}

func TestWriteTemplatesConfigFilePreservesExistingMode(t *testing.T) {
	configFilePath := filepath.Join(t.TempDir(), TemplateConfigFileName)
	if err := os.WriteFile(configFilePath, []byte("previous"), 0o640); err != nil {
		t.Fatal(err)
	}
	previousInfo, err := os.Stat(configFilePath)
	if err != nil {
		t.Fatal(err)
	}

	if err := writeTemplatesConfigFile(configFilePath, []byte("current"), os.Rename); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(configFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != previousInfo.Mode().Perm() {
		t.Fatalf("expected mode %04o, got %04o", previousInfo.Mode().Perm(), got)
	}
}

func TestWriteTemplatesConfigFilePreservesSymlink(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "managed-config.json")
	if err := os.WriteFile(targetPath, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	configFilePath := filepath.Join(t.TempDir(), TemplateConfigFileName)
	if err := os.Symlink(targetPath, configFilePath); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	if err := writeTemplatesConfigFile(configFilePath, []byte("current"), os.Rename); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(configFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected config symlink to remain")
	}
	contents, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "current" {
		t.Fatalf("expected symlink target to be updated, got %q", contents)
	}
}

func TestWriteTemplatesConfigFileCreatesDanglingSymlinkTarget(t *testing.T) {
	for _, relative := range []bool{false, true} {
		name := "absolute"
		if relative {
			name = "relative"
		}
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			targetDir := filepath.Join(parent, "managed")
			if err := os.Mkdir(targetDir, 0o755); err != nil {
				t.Fatal(err)
			}
			targetPath := filepath.Join(targetDir, "config.json")
			configFilePath := filepath.Join(parent, TemplateConfigFileName)
			linkTarget := targetPath
			if relative {
				var err error
				linkTarget, err = filepath.Rel(filepath.Dir(configFilePath), targetPath)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(linkTarget, configFilePath); err != nil {
				t.Skipf("symlinks are unavailable: %v", err)
			}

			if err := writeTemplatesConfigFile(configFilePath, []byte("current"), os.Rename); err != nil {
				t.Fatal(err)
			}
			contents, err := os.ReadFile(targetPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != "current" {
				t.Fatalf("expected dangling symlink target to be created, got %q", contents)
			}
		})
	}
}

func TestWriteTemplatesConfigFileResolvesRelativeTargetFromRealParent(t *testing.T) {
	parent := t.TempDir()
	realDirectory := filepath.Join(parent, "real", "nested")
	if err := os.MkdirAll(realDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasDirectory := filepath.Join(parent, "alias")
	if err := os.Symlink(realDirectory, aliasDirectory); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	configFilePath := filepath.Join(aliasDirectory, TemplateConfigFileName)
	if err := os.Symlink("../managed.json", configFilePath); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	expectedTarget := filepath.Join(filepath.Dir(realDirectory), "managed.json")

	if err := writeTemplatesConfigFile(configFilePath, []byte("current"), os.Rename); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(expectedTarget)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "current" {
		t.Fatalf("expected resolved target contents, got %q", contents)
	}
	if _, err := os.Stat(filepath.Join(parent, "managed.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected lexical target: %v", err)
	}
}

func TestIsCustomTemplateUsesPathBoundaries(t *testing.T) {
	templatesDir := filepath.Join(t.TempDir(), "nuclei-templates")
	cfg := &Config{}
	cfg.SetTemplatesDir(templatesDir)

	tests := []struct {
		name         string
		templatePath string
		want         bool
	}{
		{
			name:         "official template",
			templatePath: filepath.Join(templatesDir, "http", "test.yaml"),
			want:         false,
		},
		{
			name:         "official template sibling prefix",
			templatePath: filepath.Join(templatesDir+"-evil", "test.yaml"),
			want:         true,
		},
		{
			name:         "custom template",
			templatePath: filepath.Join(cfg.CustomGitHubTemplatesDirectory, "owner", "repo", "test.yaml"),
			want:         true,
		},
		{
			name:         "custom template sibling prefix",
			templatePath: filepath.Join(cfg.CustomGitHubTemplatesDirectory+"-evil", "test.yaml"),
			want:         false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := cfg.IsCustomTemplate(testCase.templatePath)
			if got != testCase.want {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}
