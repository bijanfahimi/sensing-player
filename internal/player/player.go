// Package player manages the Chrome browser instance used for displaying ads.
package player

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Player launches and manages a Chrome window.
type Player struct {
	executablePath string
	kioskMode      bool
	extraFlags     []string
	logger         *slog.Logger
	cmd            *exec.Cmd
}

// New creates a new Player.
func New(executablePath string, kioskMode bool, extraFlags []string, logger *slog.Logger) *Player {
	return &Player{
		executablePath: executablePath,
		kioskMode:      kioskMode,
		extraFlags:     extraFlags,
		logger:         logger,
	}
}

// Open launches Chrome pointing to the given URL.
// It returns without waiting for Chrome to exit.
func (p *Player) Open(ctx context.Context, url string) error {
	exe, err := p.resolveExecutable()
	if err != nil {
		return fmt.Errorf("resolving Chrome executable: %w", err)
	}

	args := p.buildArgs(url)

	p.logger.Info("launching Chrome", "executable", exe, "url", url, "args", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting Chrome: %w", err)
	}

	p.cmd = cmd

	// Give Chrome a moment to open
	time.Sleep(500 * time.Millisecond)

	return nil
}

// Wait waits for the Chrome process to exit.
func (p *Player) Wait() error {
	if p.cmd == nil {
		return nil
	}
	return p.cmd.Wait()
}

// Kill terminates the Chrome process.
func (p *Player) Kill() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

func (p *Player) buildArgs(url string) []string {
	args := []string{
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-translate",
		"--disable-infobars",
		"--disable-suggestions-ui",
		"--autoplay-policy=no-user-gesture-required",
	}

	if p.kioskMode {
		args = append(args, "--kiosk", "--fullscreen")
	}

	args = append(args, p.extraFlags...)
	args = append(args, url)

	return args
}

func (p *Player) resolveExecutable() (string, error) {
	if p.executablePath != "" {
		return p.executablePath, nil
	}

	candidates := chromeCandidates()
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path, nil
		}
		// Check if it's an absolute path that exists
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf(
		"Chrome not found; set chrome.executable_path in config.yaml. Searched: %s",
		strings.Join(candidates, ", "),
	)
}

func chromeCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"google-chrome",
			"chromium",
		}
	case "linux":
		return []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium-browser",
			"chromium",
		}
	case "windows":
		return []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	default:
		return []string{"google-chrome", "chromium"}
	}
}
