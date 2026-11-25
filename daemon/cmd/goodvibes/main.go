// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/hackclub/goodvibes/daemon/internal/config"
	"github.com/hackclub/goodvibes/daemon/internal/gitinfo"
	"github.com/hackclub/goodvibes/daemon/internal/httpapi"
	"github.com/hackclub/goodvibes/daemon/internal/sync"
	"github.com/hackclub/goodvibes/daemon/internal/tracker"
	"github.com/hackclub/goodvibes/daemon/internal/vault"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "daemon":
		cmdDaemon(os.Args[2:])
	case "login":
		cmdLogin(os.Args[2:])
	case "status":
		cmdStatus()
	case "emit":
		cmdEmit(os.Args[2:])
	case "wrap":
		cmdWrap(os.Args[2:])
	case "bundle":
		cmdBundle(os.Args[2:])
	case "doctor":
		cmdDoctor()
	case "version":
		fmt.Printf("goodvibes %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`goodvibes — Passive Productive Struggle Time Tracker for Vibecoding (AGPLv3)

Usage:
  goodvibes daemon              Start the lightweight background tracker daemon
  goodvibes login [flags]       Configure server URL and API key
  goodvibes status              Check live daemon status & current session stats
  goodvibes emit [flags]        Emit an activity event from any hook/plugin
  goodvibes wrap -- <cmd...>    Wrap a CLI AI agent (tracks prompt typing/VTT, excludes agent time)
  goodvibes bundle [flags]      Export (and optionally upload) a verifiable Audit Bundle
  goodvibes doctor              Verify local setup, vault integrity, and server connection
  goodvibes version             Print version info`)
}

func cmdDaemon(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	listen := fs.String("listen", "", "Listen address (default from config, 127.0.0.1:42420)")
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		fatal("load config: %v", err)
	}
	if *listen != "" {
		cfg.ListenAddr = *listen
	}

	v, err := vault.Open(cfg.DataDir)
	if err != nil {
		fatal("open local vault: %v", err)
	}

	syncClient := syncer.New(cfg)
	tr := tracker.New(v, cfg.IdleTimeoutSeconds, func(hb tracker.Heartbeat) {
		syncClient.Enqueue(hb)
	})

	// Idle & background sync ticker (very low CPU — wakes once every 10s)
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				tr.TickIdle(now.Unix())
				_ = syncClient.Flush()
			case <-stopCh:
				return
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpapi.NewHandler(tr, syncClient),
		ReadHeaderTimeout: 5 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		close(stopCh)
		_ = syncClient.Flush()
		_ = srv.Close()
	}()

	fmt.Printf("goodvibes daemon v%s listening on http://%s (vault: %s, server: %s)\n",
		version, cfg.ListenAddr, filepath.Join(cfg.DataDir, "vault"), cfg.ServerURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal("daemon server error: %v", err)
	}
}

func cmdLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	serverURL := fs.String("server", "http://127.0.0.1:3000", "GoodVibes server URL")
	apiKey := fs.String("api-key", "", "Personal API key from dashboard (gv_live_...)")
	username := fs.String("username", "", "Your GoodVibes username")
	_ = fs.Parse(args)

	cfg, _ := config.Load()
	cfg.ServerURL = strings.TrimRight(*serverURL, "/")
	if *apiKey != "" {
		cfg.APIKey = *apiKey
	}
	if *username != "" {
		cfg.Username = *username
	}
	if err := config.Save(cfg); err != nil {
		fatal("save config: %v", err)
	}
	fmt.Printf("Saved GoodVibes config to %s\n", config.ConfigPath())
}

func cmdStatus() {
	cfg, _ := config.Load()
	resp, err := http.Get("http://" + cfg.ListenAddr + "/v1/health")
	if err != nil {
		fmt.Printf("Daemon is NOT running on %s (%v)\nRun `goodvibes daemon` to start passive tracking.\n", cfg.ListenAddr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var data map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&data)
	out, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(out))
}

func cmdEmit(args []string) {
	fs := flag.NewFlagSet("emit", flag.ExitOnError)
	evType := fs.String("type", "pulse", "Event type: start|pulse|stop|prompt_submitted|agent_start|agent_stop")
	cat := fs.String("category", "prompt_typing", "Category: prompt_typing|prompt_vtt|human_editing|code_review|testing|agent_working")
	duration := fs.Int("duration", 0, "Explicit duration in seconds")
	harness := fs.String("harness", "cli", "AI harness name (claude-code, cursor, codex, vtt, etc.)")
	model := fs.String("model", "", "Model name (optional)")
	lang := fs.String("language", "", "Primary language (optional)")
	project := fs.String("project", "", "Project name override (auto-detected from git if omitted)")
	cwd := fs.String("cwd", "", "Working directory (defaults to current dir)")
	prompt := fs.String("prompt", "", "Prompt text (stays LOCAL ONLY for metrics & scrubbed vault)")
	keystrokes := fs.Int("keystrokes", 0, "Keystroke count")
	vttWords := fs.Int("vtt-words", 0, "Voice-to-text spoken word count")
	_ = fs.Parse(args)

	workDir := *cwd
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	ev := tracker.Event{
		Type:        *evType,
		Category:    tracker.ActivityCategory(*cat),
		DurationSec: *duration,
		Cwd:         workDir,
		Project:     *project,
		Harness:     *harness,
		Model:       *model,
		Language:    *lang,
		PromptText:  *prompt,
		Keystrokes:  *keystrokes,
		VTTWords:    *vttWords,
	}

	cfg, _ := config.Load()
	body, _ := json.Marshal(ev)
	resp, err := http.Post("http://"+cfg.ListenAddr+"/v1/events", "application/json", bytes.NewReader(body))
	if err != nil {
		// Fallback: record directly to local vault if daemon isn't running
		v, verr := vault.Open(cfg.DataDir)
		if verr != nil {
			fatal("daemon unreachable and vault open failed: %v", verr)
		}
		sc := syncer.New(cfg)
		tr := tracker.New(v, cfg.IdleTimeoutSeconds, func(hb tracker.Heartbeat) {
			sc.Enqueue(hb)
		})
		tr.ProcessEvent(ev)
		_ = sc.Flush()
		return
	}
	defer resp.Body.Close()
}

// cmdWrap wraps any interactive CLI agent prompt loop:
// It measures the time the Hack Clubber spends composing each prompt (typing or VTT)
// and explicitly marks the agent execution phase as `agent_working` (0s billable human time!).
func cmdWrap(args []string) {
	fs := flag.NewFlagSet("wrap", flag.ExitOnError)
	harness := fs.String("harness", "cli-agent", "Harness name (e.g. claude-code, codex, aider, gemini-cli)")
	model := fs.String("model", "", "Model name")
	mode := fs.String("mode", "prompt_typing", "Input mode: prompt_typing or prompt_vtt")
	_ = fs.Parse(args)

	cmdArgs := fs.Args()
	if len(cmdArgs) == 0 {
		fatal("usage: goodvibes wrap [--harness claude-code] [--mode prompt_typing|prompt_vtt] -- <agent-command> [args...]")
	}

	cwd, _ := os.Getwd()
	repo := gitinfo.Detect(cwd)
	fmt.Printf("[goodvibes] Wrapping %s on project %q (mode=%s). Agent execution time is EXCLUDED.\n",
		*harness, repo.ProjectName, *mode)
	fmt.Println("[goodvibes] Type or dictate your prompt and press Enter (empty line or Ctrl-D to quit):")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nprompt> ")
		composeStart := time.Now()
		if !scanner.Scan() {
			break
		}
		promptText := strings.TrimSpace(scanner.Text())
		if promptText == "" {
			continue
		}
		composeDur := int(time.Since(composeStart).Seconds())
		if composeDur < 1 {
			composeDur = 1
		}

		// 1. Emit human prompt composition time (billable productive struggle)
		sendEventLocal(tracker.Event{
			Type:        "prompt_submitted",
			Category:    tracker.ActivityCategory(*mode),
			DurationSec: composeDur,
			Cwd:         cwd,
			Project:     repo.ProjectName,
			Harness:     *harness,
			Model:       *model,
			PromptText:  promptText,
			Keystrokes:  len(promptText),
		})

		// 2. Run the agent command — tracked as `agent_working` (NOT billable!)
		agentStart := time.Now()
		fullArgs := append(cmdArgs[1:], promptText)
		cmd := exec.Command(cmdArgs[0], fullArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		_ = cmd.Run()

		agentDur := int(time.Since(agentStart).Seconds())
		if agentDur < 1 {
			agentDur = 1
		}
		sendEventLocal(tracker.Event{
			Type:        "agent_stop",
			Category:    tracker.CatAgentWorking,
			DurationSec: agentDur,
			Cwd:         cwd,
			Project:     repo.ProjectName,
			Harness:     *harness,
			Model:       *model,
		})
		fmt.Printf("\n[goodvibes] Logged %ds human (%s) | Excluded %ds agent working\n", composeDur, *mode, agentDur)
	}
}

func sendEventLocal(ev tracker.Event) {
	cfg, _ := config.Load()
	body, _ := json.Marshal(ev)
	resp, err := http.Post("http://"+cfg.ListenAddr+"/v1/events", "application/json", bytes.NewReader(body))
	if err == nil {
		_ = resp.Body.Close()
		return
	}
	v, err := vault.Open(cfg.DataDir)
	if err != nil {
		return
	}
	sc := syncer.New(cfg)
	tr := tracker.New(v, cfg.IdleTimeoutSeconds, func(hb tracker.Heartbeat) {
		sc.Enqueue(hb)
	})
	tr.ProcessEvent(ev)
	_ = sc.Flush()
}

func cmdBundle(args []string) {
	fs := flag.NewFlagSet("bundle", flag.ExitOnError)
	project := fs.String("project", "", "Project name to export (required)")
	program := fs.String("program", "YSWS", "Hack Club program name (e.g. Stardance, High Seas, YSWS)")
	outFile := fs.String("out", "", "Write audit bundle JSON to file")
	upload := fs.Bool("upload", false, "Upload audit bundle to central server for reviewer verification")
	notes := fs.String("notes", "", "Optional reflection / ship notes for reviewers")
	_ = fs.Parse(args)

	if *project == "" {
		cwd, _ := os.Getwd()
		*project = gitinfo.Detect(cwd).ProjectName
	}

	cfg, err := config.Load()
	if err != nil {
		fatal("load config: %v", err)
	}
	v, err := vault.Open(cfg.DataDir)
	if err != nil {
		fatal("open vault: %v", err)
	}
	bundle, err := v.ExportBundle(*project, 0, 0)
	if err != nil {
		fatal("export bundle: %v", err)
	}

	raw, _ := json.MarshalIndent(bundle, "", "  ")
	if *outFile != "" {
		if err := os.WriteFile(*outFile, raw, 0o600); err != nil {
			fatal("write bundle: %v", err)
		}
		fmt.Printf("Wrote verifiable Audit Bundle (%d entries, chain_valid=%v) to %s\n",
			bundle.EntryCount, bundle.ChainValid, *outFile)
	}

	if *upload {
		if cfg.APIKey == "" {
			fatal("API key not configured. Run `goodvibes login --api-key ...` first.")
		}
		payload, _ := json.Marshal(map[string]any{
			"project": *project,
			"program": *program,
			"notes":   *notes,
			"bundle":  bundle,
		})
		req, _ := http.NewRequest("POST", cfg.ServerURL+"/api/v1/submissions", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			fatal("upload failed: %v", err)
		}
		defer resp.Body.Close()
		var res map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&res)
		out, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(out))
		return
	}

	if *outFile == "" {
		fmt.Println(string(raw))
	}
}

func cmdDoctor() {
	cfg, _ := config.Load()
	fmt.Printf("GoodVibes Doctor v%s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  Config path : %s\n", config.ConfigPath())
	fmt.Printf("  Server URL  : %s\n", cfg.ServerURL)
	fmt.Printf("  API key set : %v\n", cfg.APIKey != "")
	fmt.Printf("  Privacy mode: %s\n", cfg.PrivacyMode)

	v, err := vault.Open(cfg.DataDir)
	if err != nil {
		fmt.Printf("  Local vault : ERROR (%v)\n", err)
	} else {
		b, _ := v.ExportBundle("", 0, 0)
		fmt.Printf("  Local vault : OK (%d entries, hash chain valid=%v)\n", b.EntryCount, b.ChainValid)
	}

	resp, err := http.Get("http://" + cfg.ListenAddr + "/v1/health")
	if err != nil {
		fmt.Printf("  Daemon      : STOPPED (%s)\n", cfg.ListenAddr)
	} else {
		_ = resp.Body.Close()
		fmt.Printf("  Daemon      : RUNNING (%s)\n", cfg.ListenAddr)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
