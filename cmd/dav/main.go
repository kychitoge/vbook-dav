package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"vbook-dav/internal/config"
	"vbook-dav/internal/notify"
	"vbook-dav/internal/r2sync"
	"vbook-dav/internal/storage/local"
	"vbook-dav/internal/webdav"
)

const (
	defaultReadTimeout     = 30 * time.Minute // Accommodates multi-megabyte backup archives on slower connections
	defaultWriteTimeout    = 30 * time.Minute
	defaultIdleTimeout     = 120 * time.Second
	defaultShutdownTimeout = 5 * time.Second
	defaultDirPermissions  = 0755
)

type LANAddress struct {
	InterfaceName string
	IP            string
}

func main() {
	// Initialize Windows console (UTF-8 code page & Virtual Terminal escape processing)
	initTerminal()

	// Use clean HH:MM:SS timestamps without cluttered date prefixes
	log.SetFlags(log.Ltime)

	cfg := config.Load()

	// Warn if default credentials are in use
	if cfg.Password == "vbook123" {
		log.Println("[warn ] Default password 'vbook123' is in use. Set DAV_PASS in your environment configuration.")
	}

	// Initialize Local Storage Engine
	st, err := local.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("[fail ] Could not initialize storage directory: %v. Verify filesystem permissions.", err)
	}

	// Purge any orphaned temporary files left by interrupted uploads from previous runs
	if cleaned := st.CleanupTempFiles(); cleaned > 0 {
		log.Printf("[info ] Removed %d temporary upload file(s) from previous session", cleaned)
	}

	// Pre-create backup folder to ensure initial connectivity checks succeed
	backupDir := filepath.Join(cfg.DataDir, cfg.BackupFolder)
	if err := os.MkdirAll(backupDir, defaultDirPermissions); err != nil {
		log.Printf("[warn ] Could not create default backup directory at %s: %v. Verify directory permissions.", backupDir, err)
	}

	// Initialize WebDAV Handler & Middleware
	handler := webdav.NewHandler(st)
	authMW := webdav.NewAuthMiddleware(cfg.Username, cfg.Password, cfg.ReadOnly)
	wrappedHandler := authMW.Wrap(handler)

	// Initialize Optional Discord Notifier (Null Object Pattern ensures safe fallback)
	notifier := notify.NewDiscord(cfg.DiscordWebhookURL)

	// Initialize Cloudflare R2 Syncer
	var syncer *r2sync.Syncer
	syncerCtx, syncerCancel := context.WithCancel(context.Background())
	defer syncerCancel()

	if cfg.HasR2() {
		syncer = r2sync.New(cfg.R2Endpoint, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Bucket, cfg.DataDir)
	}

	// Wire background synchronization hooks
	if syncer != nil {
		syncer.OnDone(func(job r2sync.Job, err error) {
			if err != nil {
				go notifier.Send(notify.Message{
					Title:   "R2 Sync Failed",
					Body:    fmt.Sprintf("Key: %s\nError: %v", job.R2Key, err),
					IsError: true,
				})
			} else {
				go notifier.Send(notify.Message{
					Title:   "Backup Synced to R2",
					Body:    fmt.Sprintf("Key: %s", job.R2Key),
					IsError: false,
				})
			}
		})

		handler.SetSyncHooks(
			func(relPath string) {
				cleanRel := r2sync.NormalizeKey(relPath)
				localPath := filepath.Join(cfg.DataDir, filepath.FromSlash(cleanRel))
				syncer.Enqueue(r2sync.Job{
					Action:    r2sync.ActionUpload,
					LocalPath: localPath,
					R2Key:     cleanRel,
				})
			},
			func(relPath string) {
				cleanRel := r2sync.NormalizeKey(relPath)
				syncer.Enqueue(r2sync.Job{
					Action: r2sync.ActionDelete,
					R2Key:  cleanRel,
				})
			},
		)

		syncer.Start(syncerCtx)
	}

	// Discover LAN IP addresses
	lanAddrs := getLANAddresses()

	// Display clean operational banner
	printBanner(cfg, lanAddrs, syncer != nil)

	server := &http.Server{
		Addr:         cfg.Port,
		Handler:      wrappedHandler,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
		IdleTimeout:  defaultIdleTimeout,
	}

	// Trap termination signals for graceful shutdown
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[ok   ] WebDAV server is listening on %s", cfg.Port)
		log.Println("[info ] Select Ctrl+C to stop the server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[fail ] Server stopped listening: %v. Check network port availability.", err)
		}
	}()

	<-stopChan
	log.Println("\n[info ] Shutting down server gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[warn ] Server shutdown did not complete cleanly: %v", err)
	}

	// Cancel syncer background worker and reconciliation loop
	syncerCancel()

	log.Println("[ok   ] Server stopped successfully")
}

func getLANAddresses() []LANAddress {
	var results []LANAddress

	ifaces, err := net.Interfaces()
	if err != nil {
		return []LANAddress{{InterfaceName: "Localhost", IP: "127.0.0.1"}}
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		nameLower := strings.ToLower(iface.Name)
		if strings.Contains(nameLower, "radmin") ||
			strings.Contains(nameLower, "virtual") ||
			strings.Contains(nameLower, "vbox") ||
			strings.Contains(nameLower, "vmware") ||
			strings.Contains(nameLower, "bluetooth") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() {
				continue
			}

			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}

			if ip4.IsLinkLocalUnicast() {
				continue
			}

			if ip4.IsPrivate() {
				results = append(results, LANAddress{
					InterfaceName: iface.Name,
					IP:            ip4.String(),
				})
			}
		}
	}

	if len(results) == 0 {
		results = append(results, LANAddress{InterfaceName: "Localhost", IP: "127.0.0.1"})
	}
	return results
}

func isRunningInContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return false
}

func useColor() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

func printBanner(cfg *config.Config, addrs []LANAddress, replicationEnabled bool) {
	port := strings.TrimPrefix(cfg.Port, ":")

	color := useColor()
	cReset := ""
	cBold := ""
	cCyan := ""
	cGreen := ""
	cYellow := ""
	cGray := ""
	if color {
		cReset = "\033[0m"
		cBold = "\033[1m"
		cCyan = "\033[36m"
		cGreen = "\033[32m"
		cYellow = "\033[33m"
		cGray = "\033[90m"
	}

	r2Badge := cGray + "Disabled" + cReset
	if replicationEnabled {
		r2Badge = cGreen + fmt.Sprintf("Active (bucket: %s)", cfg.R2Bucket) + cReset
	}

	discordBadge := cGray + "Disabled" + cReset
	if cfg.DiscordWebhookURL != "" {
		discordBadge = cGreen + "Enabled" + cReset
	}

	modeBadge := cGreen + "Read/Write" + cReset
	if cfg.ReadOnly {
		modeBadge = cYellow + "Read-Only" + cReset
	}

	absDataDir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		absDataDir = cfg.DataDir
	}

	inContainer := isRunningInContainer()
	divider := "────────────────────────────────────────────────────────────"

	fmt.Println()
	fmt.Println(cGray + divider + cReset)
	fmt.Printf("  %svbook-dav%s  WebDAV Gateway v1.0\n", cBold+cCyan, cReset)
	fmt.Printf("  %sHigh-performance local WebDAV backup server for vBook%s\n", cGray, cReset)
	fmt.Println(cGray + divider + cReset)
	fmt.Printf("  %sConfiguration%s\n", cBold, cReset)
	fmt.Printf("  • Storage directory : %s\n", absDataDir)
	fmt.Printf("  • Authentication    : user=%s (%s)\n", cfg.Username, modeBadge)
	fmt.Printf("  • Cloud replication : %s\n", r2Badge)
	fmt.Printf("  • Discord alerts    : %s\n", discordBadge)
	fmt.Println()
	fmt.Printf("  %sNetwork endpoints%s\n", cBold, cReset)
	for _, a := range addrs {
		endpoint := fmt.Sprintf("http://%s:%s/", a.IP, port)
		extra := ""
		if inContainer {
			extra = " " + cGray + "(Container Internal)" + cReset
		}
		fmt.Printf("  • %-17s : %s%s%s%s\n", a.InterfaceName, cCyan, endpoint, cReset, extra)
	}
	if inContainer {
		fmt.Printf("  • %-17s : %shttp://<HOST_IP>:%s/%s %s(Host Machine / LAN / Tailscale)%s\n",
			"Host Access", cCyan, port, cReset, cGray, cReset)
	}
	fmt.Println()
	fmt.Printf("  %svBook app setup%s (Settings > Backup WebDAV)\n", cBold, cReset)
	if inContainer {
		fmt.Printf("  • Server URL        : %shttp://<HOST_OR_TAILSCALE_IP>:%s/%s %s(e.g. your PC IP)%s\n",
			cGreen+cBold, port, cReset, cGray, cReset)
	} else if len(addrs) > 0 {
		fmt.Printf("  • Server URL        : %shttp://%s:%s/%s\n", cGreen+cBold, addrs[0].IP, port, cReset)
	}
	fmt.Printf("  • Target folder     : %s%s%s\n", cBold, cfg.BackupFolder, cReset)
	fmt.Printf("  • Username          : %s\n", cfg.Username)
	passwordHint := cGray + "(configured in .env)" + cReset
	if cfg.Password == "vbook123" {
		passwordHint = cYellow + "(default: set DAV_PASS in .env)" + cReset
	}
	fmt.Printf("  • Password          : •••••••• %s\n", passwordHint)
	fmt.Println(cGray + divider + cReset)
	fmt.Println()
}
