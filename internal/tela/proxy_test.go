package tela

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPathWithinAcceptsNestedPath(t *testing.T) {
	root := t.TempDir()
	got, err := pathWithin(root, "app", "assets", "index.html")
	if err != nil {
		t.Fatalf("pathWithin returned error: %v", err)
	}
	want := filepath.Join(root, "app", "assets", "index.html")
	if got != want {
		t.Fatalf("pathWithin = %q, want %q", got, want)
	}
}

func TestPathWithinRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "..", "outside.txt")
	if _, err := pathWithin(root, "app", "..", "..", "outside.txt"); err == nil {
		t.Fatal("pathWithin accepted traversal outside root")
	}
	if _, err := pathWithin(root, outside); err == nil {
		t.Fatal("pathWithin accepted an absolute outside path")
	}
}

func TestPathWithinKeepsRoot(t *testing.T) {
	root := t.TempDir()
	got, err := pathWithin(root)
	if err != nil {
		t.Fatalf("pathWithin returned error: %v", err)
	}
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(got, root) {
			t.Fatalf("pathWithin root = %q, want %q", got, root)
		}
		return
	}
	if got != root {
		t.Fatalf("pathWithin root = %q, want %q", got, root)
	}
}

func TestPathWithinRejectsExistingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if _, err := pathWithin(root, "linked", "file.txt"); err == nil {
		t.Fatal("pathWithin accepted an existing symlink component")
	}
}

func TestProxyManagerShutdownBeforeStart(t *testing.T) {
	pm := NewProxyManager(0, func() string { return "" })
	pm.Shutdown()
}

func TestProxyManagerShutdownReleasesServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}

	pm := NewProxyManager(port, func() string { return "" })
	pm.Start()
	t.Cleanup(pm.Shutdown)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(2 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("proxy server did not start")
	}

	pm.Shutdown()
	rebound, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("reserve verification port: %v", err)
	}
	defer rebound.Close()
}
