package steamcmd

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

func init() {
	driver.RegisterSteamExecutor(driver.DefaultSteamExecutorDriver, func(cfg config.RegistryConfig) driver.SteamExecutor {
		return &execSteamRunner{
			bin:      cfg.SteamCMDBin,
			workDir:  cfg.WorkerWorkDir,
			proxyURL: cfg.UpstreamProxyURL,
		}
	})
}

type execSteamRunner struct {
	mu       sync.Mutex
	bin      string
	workDir  string
	proxyURL string
}

func (r *execSteamRunner) Run(ctx context.Context, installDir string, args []string, hooks driver.SteamHooks) error {
	if !r.mu.TryLock() {
		if hooks.OnWait != nil {
			hooks.OnWait()
		}
		r.mu.Lock()
	}
	defer r.mu.Unlock()
	if hooks.OnAcquired != nil {
		hooks.OnAcquired()
	}

	runtimeDir := filepath.Join(r.workDir, ".steamcmd")
	homeDir := filepath.Join(r.workDir, ".steam-home")
	_ = os.MkdirAll(runtimeDir, 0o755)
	_ = os.MkdirAll(homeDir, 0o755)
	_ = os.MkdirAll(installDir, 0o755)

	fullArgs := append([]string{"+force_install_dir", installDir, "+login", "anonymous"}, args...)
	fullArgs = append(fullArgs, "+quit")
	cmd := exec.CommandContext(ctx, r.bin, fullArgs...)
	cmd.Env = r.buildCmdEnv(homeDir, runtimeDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if hooks.OnLine != nil {
			hooks.OnLine(scanner.Text())
		}
	}
	return cmd.Wait()
}

func (r *execSteamRunner) buildCmdEnv(homeDir, runtimeDir string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + homeDir,
		"STEAMCMD_RUNTIME_DIR=" + runtimeDir,
		"STEAMCMD_HOME_DIR=" + homeDir,
	}
	if p := strings.TrimSpace(r.proxyURL); p != "" {
		env = append(env, "HTTP_PROXY="+p, "HTTPS_PROXY="+p, "http_proxy="+p, "https_proxy="+p)
	}
	return env
}
