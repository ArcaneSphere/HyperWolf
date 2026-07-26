package desktop

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func Install(exePath string, iconData []byte) error {
	switch runtime.GOOS {
	case "linux":
		return installLinux(exePath, iconData)
	case "darwin":
		return installDarwin(exePath, iconData)
	case "windows":
		return installWindows(exePath, iconData)
	}
	return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

func Uninstall() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallLinux()
	case "darwin":
		return uninstallDarwin()
	case "windows":
		return uninstallWindows()
	}
	return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}

// ---- helpers ----

func home() string {
	d, _ := os.UserHomeDir()
	return d
}

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

func writeFile(path string, data []byte, perm os.FileMode) error {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func remove(paths ...string) {
	for _, p := range paths {
		os.Remove(p)
	}
}

func removeAll(paths ...string) {
	for _, p := range paths {
		os.RemoveAll(p)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := ensureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// ---- Linux (XDG) ----

const linuxDesktop = `[Desktop Entry]
Type=Application
Name=HyperWolf
Exec=%s
Icon=hyperwolf
Categories=Network;
Terminal=false
`

const linuxAutostart = `[Desktop Entry]
Type=Application
Name=HyperWolf
Exec=%s
Icon=hyperwolf
X-GNOME-Autostart-enabled=true
NoDisplay=true
`

func installLinux(exePath string, iconData []byte) error {
	target := filepath.Join(home(), ".local", "bin", "hyperwolf")
	if err := copyFile(exePath, target); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}
	os.Chmod(target, 0755)

	if err := writeFile(filepath.Join(home(), ".local", "share", "icons", "hyperwolf.png"), iconData, 0644); err != nil {
		return fmt.Errorf("write icon: %w", err)
	}
	if err := writeFile(filepath.Join(home(), ".local", "share", "applications", "hyperwolf.desktop"), []byte(fmt.Sprintf(linuxDesktop, target)), 0644); err != nil {
		return fmt.Errorf("write desktop: %w", err)
	}
	if err := writeFile(filepath.Join(home(), ".config", "autostart", "hyperwolf.desktop"), []byte(fmt.Sprintf(linuxAutostart, target)), 0644); err != nil {
		return fmt.Errorf("write autostart: %w", err)
	}

	exec.Command("xdg-desktop-menu", "forceupdate").Run()
	return nil
}

func uninstallLinux() error {
	remove(
		filepath.Join(home(), ".local", "bin", "hyperwolf"),
		filepath.Join(home(), ".local", "share", "icons", "hyperwolf.png"),
		filepath.Join(home(), ".local", "share", "applications", "hyperwolf.desktop"),
		filepath.Join(home(), ".config", "autostart", "hyperwolf.desktop"),
	)
	exec.Command("xdg-desktop-menu", "forceupdate").Run()
	return nil
}

// ---- macOS (.app bundle + LaunchAgent) ----

const macPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>hyperwolf</string>
	<key>CFBundleIdentifier</key>
	<string>com.hyperwolf.app</string>
	<key>CFBundleName</key>
	<string>HyperWolf</string>
	<key>CFBundleIconFile</key>
	<string>icon</string>
	<key>LSUIElement</key>
	<true/>
</dict>
</plist>
`

const macAgent = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.hyperwolf</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
</dict>
</plist>
`

func installDarwin(exePath string, iconData []byte) error {
	appDir := filepath.Join(home(), "Applications", "HyperWolf.app")
	exeDir := filepath.Join(appDir, "Contents", "MacOS")
	resDir := filepath.Join(appDir, "Contents", "Resources")

	if err := ensureDir(exeDir); err != nil {
		return fmt.Errorf("bundle dir: %w", err)
	}
	if err := ensureDir(resDir); err != nil {
		return fmt.Errorf("resources dir: %w", err)
	}

	exeTarget := filepath.Join(exeDir, "hyperwolf")
	if err := copyFile(exePath, exeTarget); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}
	os.Chmod(exeTarget, 0755)

	if err := writeFile(filepath.Join(resDir, "icon.png"), iconData, 0644); err != nil {
		return fmt.Errorf("write icon: %w", err)
	}
	if err := writeFile(filepath.Join(appDir, "Contents", "Info.plist"), []byte(macPlist), 0644); err != nil {
		return fmt.Errorf("write Info.plist: %w", err)
	}
	if err := writeFile(filepath.Join(home(), "Library", "LaunchAgents", "com.hyperwolf.plist"), []byte(fmt.Sprintf(macAgent, exeTarget)), 0644); err != nil {
		return fmt.Errorf("write launchagent: %w", err)
	}

	return nil
}

func uninstallDarwin() error {
	removeAll(filepath.Join(home(), "Applications", "HyperWolf.app"))
	remove(filepath.Join(home(), "Library", "LaunchAgents", "com.hyperwolf.plist"))
	return nil
}

// ---- Windows (Start Menu shortcut + Registry autostart) ----

func localAppData() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return d
	}
	return filepath.Join(home(), "AppData", "Local")
}

func appDataPath() string {
	if d := os.Getenv("APPDATA"); d != "" {
		return d
	}
	return filepath.Join(home(), "AppData", "Roaming")
}

func installWindows(exePath string, iconData []byte) error {
	installDir := filepath.Join(localAppData(), "HyperWolf")
	if err := ensureDir(installDir); err != nil {
		return fmt.Errorf("install dir: %w", err)
	}

	exeTarget := filepath.Join(installDir, "hyperwolf.exe")
	if err := copyFile(exePath, exeTarget); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}
	if err := writeFile(filepath.Join(installDir, "hyperwolf.png"), iconData, 0644); err != nil {
		return fmt.Errorf("write icon: %w", err)
	}

	// Start Menu shortcut via PowerShell
	smDir := filepath.Join(appDataPath(), "Microsoft", "Windows", "Start Menu", "Programs", "HyperWolf")
	if err := ensureDir(smDir); err != nil {
		return fmt.Errorf("start menu dir: %w", err)
	}
	psCmd := fmt.Sprintf(
		`$WS=New-Object -ComObject WScript.Shell;$SC=$WS.CreateShortcut('%s');$SC.TargetPath='%s';$SC.WorkingDirectory='%s';$SC.Save()`,
		filepath.Join(smDir, "HyperWolf.lnk"), exeTarget, installDir,
	)
	if err := exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd).Run(); err != nil {
		return fmt.Errorf("create shortcut: %w", err)
	}

	// Autostart via registry
	regCmd := fmt.Sprintf(`REG ADD "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /V "HyperWolf" /T REG_SZ /F /D "%s"`, exeTarget)
	if err := exec.Command("cmd.exe", "/C", regCmd).Run(); err != nil {
		return fmt.Errorf("set autostart: %w", err)
	}

	return nil
}

func uninstallWindows() error {
	exec.Command("cmd.exe", "/C", `REG DELETE "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /V "HyperWolf" /F`).Run()
	remove(
		filepath.Join(appDataPath(), "Microsoft", "Windows", "Start Menu", "Programs", "HyperWolf", "HyperWolf.lnk"),
	)
	removeAll(filepath.Join(localAppData(), "HyperWolf"))
	return nil
}
