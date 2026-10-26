// Command tidemark runs the Tidemark server.
//
//	tidemark              start the server
//	tidemark healthcheck  exit 0 if the local server answers /api/health (for Docker HEALTHCHECK)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/bartvs98/tidemark/internal/ai"
	"github.com/bartvs98/tidemark/internal/api"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/config"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/icons"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/notify"
	"github.com/bartvs98/tidemark/internal/registry"
	"github.com/bartvs98/tidemark/internal/resolve"
	"github.com/bartvs98/tidemark/internal/rules"
	"github.com/bartvs98/tidemark/internal/settings"
	reposource "github.com/bartvs98/tidemark/internal/source"
	"github.com/bartvs98/tidemark/internal/stats"
	"github.com/bartvs98/tidemark/internal/store"
	tmversion "github.com/bartvs98/tidemark/internal/version"
	"github.com/bartvs98/tidemark/web"
)

// version is set at build time: -ldflags "-X main.version=1.0.0".
var version = "dev"

// startupCheckAge: at startup, check only if the last check finished longer ago than this
// (or a container has no valid result).
const startupCheckAge = time.Hour

// Caps for the after-check hook's two parts (check.afterCheckTimeout caps the whole hook).
const (
	prefetchTimeout = time.Minute
	autoAITimeout   = 2 * time.Minute
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(cfg.Addr))
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if err := run(cfg, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, log *slog.Logger) error {
	started := time.Now()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	source, err := dockerSource(cfg)
	if err != nil {
		return err
	}
	info := source.Info()
	log.Info("container source", "kind", info.Kind, "location", info.Location)

	db, err := store.Open(filepath.Join(cfg.DataDir, "tidemark.db"))
	if err != nil {
		return err
	}
	defer db.Close()

	// Outgoing requests are counted per group for the stats page; each run stores the
	// counts since the previous one.
	reqs := stats.NewRequests()
	reg := registry.New("tidemark/"+version, registry.WithTagCache(db),
		registry.WithTransport(reqs.Wrap(remote.DefaultTransport, func(*http.Request) string { return "registry" })))
	checks := check.New(source, reg, db, log)
	checks.UseRulesFile(cfg.ConfigFile)
	log.Info("config file", "path", cfg.ConfigFile)
	if err := checks.Load(ctx); err != nil {
		return err
	}

	// AI changelog summaries: off until settings configure a provider.
	aiSvc := ai.New(db, log)
	if err := aiSvc.Load(ctx); err != nil {
		return err
	}

	// Prefetch release notes for containers with an update, so the detail page is instant
	// and the notes can be scanned for breaking changes, then queue a summary when automatic
	// AI summaries are on (a follow-up: it runs once every image is checked, so a slow
	// provider doesn't hold a worker). The token comes from settings.
	changelogs := changelog.New(db, "", log, changelog.WithTransport(reqs.Wrap(http.DefaultTransport, stats.Forge)))
	checks.SetAfterCheck(func(ctx context.Context, c docker.Container, rs rules.Rules, r resolve.Result) func(context.Context) {
		if r.Class == tmversion.None {
			return nil
		}
		ref, err := imageref.Parse(c.ImageRef)
		if err != nil {
			return nil
		}
		repo, _, ok := reposource.Resolve(c, ref, rs)
		if !ok {
			return nil
		}
		var need, latest *tmversion.Version
		if v, ok := tmversion.Parse(r.Running); ok {
			need = &v
		}
		if v, ok := tmversion.Parse(r.Latest); ok {
			latest = &v
		}
		syncCtx, cancel := context.WithTimeout(ctx, prefetchTimeout)
		if since, ok := registry.FreshSinceOf(ctx); ok { // Check now: look for new notes too
			syncCtx = changelog.RecheckSince(syncCtx, since)
		}
		err = changelogs.Sync(syncCtx, repo, need, latest)
		cancel()
		if err != nil {
			log.Debug("changelog prefetch failed", "repo", repo.Key(), "err", err)
		}

		// A channel build isn't compared with stable releases, so their notes say nothing about it.
		if p, autoMin := aiSvc.Status(); p == nil || autoMin == 0 || resolve.Channel(ref.Tag) != "" {
			return nil
		}
		k, ok := ai.KeyFor(repo.Key(), need, latest)
		if !ok {
			return nil
		}
		ch, err := changelogs.Changes(ctx, repo, need, latest)
		if err != nil {
			log.Debug("stored release notes unavailable", "repo", repo.Key(), "err", err)
			return nil
		}
		req := ai.RequestFrom(ref.Familiar(), repo.Path, r.Running, r.Latest, ch)
		if !aiSvc.WantsAuto(k, req) {
			return nil
		}
		return func(ctx context.Context) {
			aiCtx, cancel := context.WithTimeout(ctx, autoAITimeout)
			defer cancel()
			aiSvc.Auto(aiCtx, k, req)
		}
	})

	// Notifications: one Telegram message per check run with the new updates, or a digest.
	// The service always exists; settings turn it on and off.
	// After a run: delete the stored AI summaries no container needs any more (AI on or
	// off), record the run's stats, then notify. apiSrv is set below, before the first run.
	notifier := notify.New(db, notify.Options{Log: log})
	var apiSrv *api.Server
	checks.SetAfterRun(func(ctx context.Context, run check.Finished) {
		if keys, err := apiSrv.AIKeys(ctx); err != nil {
			log.Debug("AI summary cleanup skipped", "err", err)
		} else if n, err := aiSvc.Prune(ctx, keys); err != nil {
			log.Warn("AI summary cleanup failed", "err", err)
		} else if n > 0 {
			log.Info("AI summaries cleaned up", "deleted", n)
		}
		if err := recordRunStats(ctx, db, apiSrv, run, reqs, changelogs); err != nil {
			log.Warn("recording the run's stats failed", "err", err)
		}
		notifier.AfterRun(ctx)
	})

	// Editable settings (env var > stored > default) are pushed into the services by Start
	// and after every change in the settings dialog. The manager runs appliers one at a
	// time, so their state needs no lock. Icons go to the API server, built below.
	setIcons := func(svc *icons.Service) { apiSrv.SetIcons(svc) }
	settingsMgr := settings.New(db, cfg.Env, cfg.Editable, appliers(ctx, cfg, log, checks, changelogs, notifier, aiSvc, setIcons), log)

	// The API server is built before the startup check so the first run's hook has its source.
	apiSrv = api.New(api.Deps{
		Version:    version,
		Log:        log,
		Source:     source,
		Checks:     checks,
		Changelogs: changelogs,
		Store:      db,
		Notify:     notifier,
		Settings:   settingsMgr,
		AI:         aiSvc,
		ConfigFile: cfg.ConfigFile,
		Background: ctx,
		Started:    started,
	})
	notifier.UseSource(apiSrv.Updates)

	if err := settingsMgr.Start(ctx); err != nil {
		return err
	}
	log.Info("settings", "editable", cfg.Editable)

	// Check at startup unless a recent run covered every container, so restarts don't hammer
	// registries. Containers without a valid result (new, recreated, or with changed rules)
	// would otherwise show nothing until the next scheduled run. A run that failed or was
	// cut short at shutdown didn't cover them.
	last := checks.LastFinished()
	unchecked, err := checks.Unchecked(ctx)
	if err != nil {
		log.Warn("can't list containers at startup", "err", err)
	}
	if time.Since(last) > startupCheckAge || unchecked > 0 || checks.Status().Err != "" {
		checks.Start(ctx, check.TriggerStartup)
	} else {
		log.Info("skipping startup check, last one is recent", "finished", last.Format(time.RFC3339))
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           apiSrv.Handler(web.SPA()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("tidemark listening", "addr", cfg.Addr, "version", version)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	// The check run was cancelled with ctx; let it write its finish time before the DB closes.
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelWait()
	if err := checks.Wait(waitCtx); err != nil {
		log.Warn("a check run was still busy at shutdown", "err", err)
	}
	return nil
}

// appliers connect the settings manager to the running services. Each one only acts
// when its part of the settings changed, and logs the new state (never a secret).
func appliers(ctx context.Context, cfg config.Config, log *slog.Logger, checks *check.Service,
	changelogs *changelog.Service, notifier *notify.Service, aiSvc *ai.Service, setIcons func(*icons.Service)) settings.Appliers {
	var (
		iconsURL    string             // URL of the running icon service, "" when off
		stopIcons   context.CancelFunc // stops its index refresh
		notifySetup *config.Runtime    // last notification settings applied
		aiSetup     *config.Runtime    // last AI settings applied
	)
	return settings.Appliers{
		Schedule: func(spec string) error {
			if spec == checks.Schedule() {
				return nil
			}
			if err := checks.SetSchedule(ctx, spec); err != nil {
				return err
			}
			if spec == "" {
				log.Info("check schedule", "spec", "off")
			} else {
				log.Info("check schedule", "spec", spec, "next", checks.Status().NextRunAt)
			}
			return nil
		},
		GitHubToken: changelogs.SetToken,
		Icons: func(on bool, url string) {
			if !on {
				url = ""
			}
			if url == iconsURL {
				return
			}
			if stopIcons != nil {
				stopIcons()
				stopIcons = nil
			}
			iconsURL = url
			if url == "" {
				setIcons(nil)
				log.Info("icons", "on", false)
				return
			}
			// A new service per URL: its cache holds the index of the collection it came from.
			iconCtx, cancel := context.WithCancel(ctx)
			svc := icons.New(url, filepath.Join(cfg.DataDir, "icons"), log)
			svc.Start(iconCtx)
			setIcons(svc)
			stopIcons = cancel
			log.Info("icons", "on", true, "url", url)
		},
		Notify: func(rt config.Runtime) error {
			if notifySetup != nil && rt.TelegramBotToken == notifySetup.TelegramBotToken && rt.TelegramChatID == notifySetup.TelegramChatID &&
				rt.NotifyLevel == notifySetup.NotifyLevel && rt.NotifyDigest == notifySetup.NotifyDigest && rt.PublicURL == notifySetup.PublicURL {
				return nil
			}
			var n notify.Notifier
			if rt.Telegram() {
				n = notify.NewTelegram(rt.TelegramBotToken, rt.TelegramChatID)
			}
			if err := notifier.Configure(ctx, n, rt.NotifyLevel, rt.PublicURL, rt.NotifyDigest); err != nil {
				return err
			}
			notifySetup = &rt
			if n == nil {
				log.Info("notifications", "on", false)
			} else {
				log.Info("notifications", "notifier", n.Name(), "level", rt.NotifyLevel.String(), "digest", rt.NotifyDigest)
			}
			return nil
		},
		AI: func(rt config.Runtime) error {
			if aiSetup != nil && rt.AI() == aiSetup.AI() && rt.AIProvider == aiSetup.AIProvider && rt.AIAPIKey == aiSetup.AIAPIKey &&
				rt.AIModel == aiSetup.AIModel && rt.AIBaseURL == aiSetup.AIBaseURL && rt.AIEffort == aiSetup.AIEffort &&
				rt.AIAutoMin == aiSetup.AIAutoMin {
				return nil
			}
			if !rt.AI() {
				aiSvc.Configure(nil, 0)
				aiSetup = &rt
				log.Info("AI summaries", "on", false)
				return nil
			}
			p, err := ai.NewProvider(rt.AIProvider, rt.AIAPIKey, rt.AIModel, rt.AIBaseURL, rt.AIEffort)
			if err != nil {
				aiSvc.Configure(nil, 0) // not the previous provider: the settings say otherwise now
				aiSetup = nil
				return err
			}
			aiSvc.Configure(p, rt.AIAutoMin)
			aiSetup = &rt
			log.Info("AI summaries", "provider", p.Name(), "model", p.Model(), "effort", p.Effort(), "auto_min_releases", rt.AIAutoMin)
			return nil
		},
	}
}

// recordRunStats stores what the dashboard showed at the end of run, with the requests
// made since the previous row and the last-known GitHub quota (no request is made). When
// the counts can't be taken, the requests stay counted for the next row.
func recordRunStats(ctx context.Context, db *store.Store, apiSrv *api.Server, run check.Finished, reqs *stats.Requests, changelogs *changelog.Service) error {
	if run.ID == 0 {
		return errors.New("the run wasn't recorded")
	}
	rs, err := apiSrv.RunStats(ctx)
	if err != nil {
		return err
	}
	rs.RunID, rs.At, rs.Duration = run.ID, run.FinishedAt, run.FinishedAt.Sub(run.StartedAt)
	var commit func()
	rs.Requests, commit = reqs.Since()
	if q, ok := changelogs.LastQuota(); ok {
		rs.GitHubRemaining, rs.GitHubLimit = &q.Remaining, &q.Limit
	}
	// The run's context may be cancelled at shutdown; the row is still worth keeping.
	if err := db.AddRunStats(context.WithoutCancel(ctx), rs); err != nil {
		return err
	}
	commit()
	return nil
}

func dockerSource(cfg config.Config) (docker.Source, error) {
	if cfg.DockerSnapshot != "" {
		return docker.NewSnapshot(cfg.DockerSnapshot), nil
	}
	return docker.NewClient(cfg.DockerHost)
}

func healthcheck(addr string) int {
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + host + "/api/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy:", resp.Status)
		return 1
	}
	return 0
}
