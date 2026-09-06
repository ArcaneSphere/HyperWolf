package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLinuxInstallAndUninstall(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux desktop integration test")
	}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	source := filepath.Join(homeDir, "source-hyperwolf")
	if err := os.WriteFile(source, []byte("binary"), 0755); err != nil {
		t.Fatalf("create source binary: %v", err)
	}
	icon := []byte("png")

	if err := installLinux(source, icon); err != nil {
		t.Fatalf("installLinux: %v", err)
	}

	installed, err := os.ReadFile(filepath.Join(homeDir, ".local", "bin", "hyperwolf"))
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(installed) != "binary" {
		t.Fatalf("installed binary = %q, want binary", installed)
	}
	installedIcon, err := os.ReadFile(filepath.Join(homeDir, ".local", "share", "icons", "hyperwolf.png"))
	if err != nil {
		t.Fatalf("read installed icon: %v", err)
	}
	if string(installedIcon) != "png" {
		t.Fatalf("installed icon = %q, want png", installedIcon)
	}

	desktopPath := filepath.Join(homeDir, ".local", "share", "applications", "hyperwolf.desktop")
	desktopEntry, err := os.ReadFile(desktopPath)
	if err != nil {
		t.Fatalf("read desktop entry: %v", err)
	}
	if !strings.Contains(string(desktopEntry), "Exec="+filepath.Join(homeDir, ".local", "bin", "hyperwolf")) {
		t.Fatalf("desktop entry does not reference installed binary: %s", desktopEntry)
	}

	if err := uninstallLinux(); err != nil {
		t.Fatalf("uninstallLinux: %v", err)
	}
	for _, path := range []string{
		filepath.Join(homeDir, ".local", "bin", "hyperwolf"),
		filepath.Join(homeDir, ".local", "share", "icons", "hyperwolf.png"),
		desktopPath,
		filepath.Join(homeDir, ".config", "autostart", "hyperwolf.desktop"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("artifact %q still exists or returned unexpected error: %v", path, err)
		}
	}
}
