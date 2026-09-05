// Command bastiondeck is the single-binary control-plane daemon: web API,
// local unix-socket control plane, scheduler, metrics and tunnel recovery.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"bastiondeck/internal/agentconn"
	"bastiondeck/internal/audit"
	"bastiondeck/internal/auth"
	"bastiondeck/internal/backup"
	"bastiondeck/internal/bootstrap"
	"bastiondeck/internal/config"
	"bastiondeck/internal/connector"
	"bastiondeck/internal/control"
	"bastiondeck/internal/credentials"
	"bastiondeck/internal/httpx"
	"bastiondeck/internal/inventory"
	"bastiondeck/internal/jobs"
	"bastiondeck/internal/metricsx"
	"bastiondeck/internal/realtime"
	"bastiondeck/internal/settings"
	"bastiondeck/internal/snippets"
	"bastiondeck/internal/sshlite"
	"bastiondeck/internal/store"
	"bastiondeck/internal/tunnel"
	"bastiondeck/internal/validate"
	"bastiondeck/internal/vault"
	"bastiondeck/internal/version"
)

func main() {
	var (
		flagVersion = flag.Bool("version", false, "print version and exit")
		flagDoctor  = flag.Bool("doctor", false, "run self checks against the data dir and exit")
	)
	flag.Parse()
	if *flagVersion {
		fmt.Println(version.String())
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ensureDirs(cfg)

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.DB.Close()

	vlt, err := vault.Load(cfg.MasterKeyHex, cfg.DataDir)
	if err != nil {
		log.Fatalf("vault: %v", err)
	}
	logs := audit.New(st.DB)
	authSvc := auth.NewService(st.DB, vlt, cfg.SessionTTL)
	boot := bootstrap.New(authSvc, logs)
	creds := credentials.New(st.DB, vlt)
	hosts := inventory.NewRepo(st.DB)
	snips := snippets.New(st.DB)
	hub := realtime.NewHub()
	agents := agentconn.New(st.DB)

	dialer := &sshlite.Dialer{Hosts: hosts, Creds: creds, DialTimeout: 10 * time.Second}
	pool := sshlite.NewPool(dialer, cfg.IdleConns)
	mgr := &connector.Manager{Hosts: hosts, SSH: pool, Agents: agents.NewProvider()}

	jobRepo := jobs.NewRepo(st.DB)
	engine := jobs.NewEngine(jobRepo, st.DB, mgr, hub, logs, cfg.ArtifactDir("runs"), cfg.MaxOutputB)
	sched := jobs.NewScheduler(engine, jobRepo)
	tunnels := tunnel.New(st.DB, pool)
	metrics := metricsx.New(st.DB, mgr)
	settingsSvc := settings.New(st.DB)
	backupSvc := backup.New(st.DB, st.Path)

	if *flagDoctor {
		rep := validate.Run(context.Background(), validate.Input{
			Store: st, Audit: logs, Cfg: cfg, Hub: hub, Version: version.Short()})
		fmt.Printf("doctor ok=%v checks=%d\n", rep.OK, len(rep.Checks))
		for _, c := range rep.Checks {
			mark := "ok"
			if !c.OK {
				mark = "FAIL"
			}
			fmt.Printf("  [%s] %s %s\n", mark, c.Name, c.Detail)
		}
		if !rep.OK {
			os.Exit(1)
		}
		return
	}

	// Recovery after restart: orphaned runs become "lost"; tunnels re-surface.
	if n, err := engine.Reconcile(context.Background()); err != nil {
		log.Printf("reconcile: %v", err)
	} else if n > 0 {
		log.Printf("reconciled %d orphaned target(s) to lost", n)
	}
	if err := tunnels.Recover(context.Background()); err != nil {
		log.Printf("tunnel recover: %v", err)
	}

	srv := httpx.New(httpx.Deps{
		Cfg: cfg, Store: st, Vault: vlt, Auth: authSvc, Audit: logs, Bootstrap: boot,
		Creds: creds, Hosts: hosts, Snippets: snips, Jobs: engine, JobRepo: jobRepo,
		Tunnels: tunnels, Metrics: metrics, Hub: hub, Connector: mgr, Scheduler: sched,
		Agents: agents, Backup: backupSvc, Settings: settingsSvc, Version: version.Short(),
	})

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sched.Run(rootCtx)
	go backgroundLoops(rootCtx, metrics, authSvc)

	httpSrv := srv.ServerConfig(cfg.Listen)
	go func() {
		log.Printf("%s listening on %s (data=%s)", version.Short(), cfg.Listen, cfg.DataDir)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	// Local-only control plane over unix socket.
	ctl := control.New(cfg.ControlSock, control.Hooks{
		Status: func() any {
			req, _ := boot.SetupRequired(context.Background())
			return map[string]any{"version": version.Short(), "setupRequired": req,
				"pool": pool.Stats()}
		},
		Doctor: func() any {
			return validate.Run(context.Background(), validate.Input{
				Store: st, Audit: logs, Cfg: cfg, Hub: hub, Version: version.Short()})
		},
		Shutdown: func(string) error { cancel(); return nil },
		RotateKey: func(hexKey string) error {
			dst, err := vault.FromHex(hexKey)
			if err != nil {
				return err
			}
			n, err := creds.RotateVault(context.Background(), dst)
			if err != nil {
				return err
			}
			log.Printf("rotated master key for %d credential(s)", n)
			return nil
		},
	})
	if err := ctl.Listen(); err != nil {
		log.Printf("control socket disabled: %v", err)
	} else {
		go func() {
			if err := ctl.Serve(); err != nil && err != http.ErrServerClosed {
				log.Printf("control: %v", err)
			}
		}()
	}

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down")
	shutdownCtx, shCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	_ = ctl.Close()
	tunnels.StopAll()
	pool.CloseAll()
	cancel()
}

func ensureDirs(cfg *config.Config) {
	for _, kind := range []string{"runs", "transfers", "backups", "tmp"} {
		if err := os.MkdirAll(filepath.Join(cfg.DataDir, kind), 0o700); err != nil {
			log.Fatalf("mkdir %s: %v", kind, err)
		}
	}
}

// backgroundLoops periodically collect metrics and prune old series.
func backgroundLoops(ctx context.Context, c *metricsx.Collector, authSvc *auth.Service) {
	metricsTick := time.NewTicker(5 * time.Minute)
	defer metricsTick.Stop()
	// 登录限流窗口清理：此前 PruneAttempts 无人调用，login_attempts 会无限增长。
	attemptTick := time.NewTicker(time.Hour)
	defer attemptTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-metricsTick.C:
			if _, err := c.Prune(ctx); err != nil {
				log.Printf("metrics prune: %v", err)
			}
		case <-attemptTick.C:
			if err := authSvc.PruneAttempts(ctx); err != nil {
				log.Printf("login attempts prune: %v", err)
			}
		}
	}
}
