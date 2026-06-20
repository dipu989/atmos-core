package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/dipu/atmos-core/config"
	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
	actrepo "github.com/dipu/atmos-core/internal/activity/repository"
	actservice "github.com/dipu/atmos-core/internal/activity/service"
	authrepo "github.com/dipu/atmos-core/internal/auth/repository"
	devrepo "github.com/dipu/atmos-core/internal/device/repository"
	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	emirepo "github.com/dipu/atmos-core/internal/emission/repository"
	emiservice "github.com/dipu/atmos-core/internal/emission/service"
	gmailrepo "github.com/dipu/atmos-core/internal/gmail/repository"
	gmailservice "github.com/dipu/atmos-core/internal/gmail/service"
	idrepo "github.com/dipu/atmos-core/internal/identity/repository"
	idservice "github.com/dipu/atmos-core/internal/identity/service"
	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	insightrepo "github.com/dipu/atmos-core/internal/insight/repository"
	insightservice "github.com/dipu/atmos-core/internal/insight/service"
	notifservice "github.com/dipu/atmos-core/internal/notification/service"
	timelineagg "github.com/dipu/atmos-core/internal/timeline/aggregator"
	timelinerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	timelineservice "github.com/dipu/atmos-core/internal/timeline/service"
	"github.com/dipu/atmos-core/platform/database"
	"github.com/dipu/atmos-core/platform/eventbus"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/dipu/atmos-core/platform/push"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger.Init(cfg.App.Env)
	defer logger.Sync()
	log := logger.L()

	db, err := database.Connect(&cfg.DB)
	if err != nil {
		log.Fatal("database connection failed", zap.Error(err))
	}

	bus := eventbus.NewInMemoryBus()

	// --- FCM (nil when not configured) ---
	var fcmSender push.Sender
	if cfg.Push.FCMServiceAccountJSON != "" {
		fcmSender, err = push.NewFCMSender(context.Background(), cfg.Push.FCMServiceAccountJSON)
		if err != nil {
			log.Fatal("FCM client init failed", zap.Error(err))
		}
	}

	// --- Repositories ---
	userRepo := idrepo.NewUserRepository(db)
	tokenRepo := authrepo.NewTokenRepository(db)
	deviceRepo := devrepo.NewDeviceRepository(db)
	activityRepo := actrepo.NewActivityRepository(db)
	emissionRepo := emirepo.NewEmissionRepository(db)
	summaryRepo := timelinerepo.NewSummaryRepository(db)
	insightRepo := insightrepo.NewInsightRepository(db)
	gmailConnRepo := gmailrepo.NewConnectionRepository(db)
	gmailLogRepo := gmailrepo.NewLogRepository(db)
	gmailProvRepo := gmailrepo.NewProviderRepository(db)

	// --- Services ---
	identitySvc := idservice.NewIdentityService(userRepo, tokenRepo)
	activitySvc := actservice.NewActivityService(activityRepo, bus)
	regionFn := func(ctx context.Context, userID uuid.UUID) string {
		prefs, err := userRepo.GetPreferences(ctx, userID)
		if err != nil || prefs.Region == "" {
			return "IN"
		}
		return prefs.Region
	}
	emissionSvc := emiservice.NewEmissionService(emissionRepo, activityRepo, bus, regionFn)
	agg := timelineagg.NewAggregator(summaryRepo)
	timelineSvc := timelineservice.NewTimelineService(summaryRepo, agg)
	insightSvc := insightservice.NewInsightService(insightRepo, summaryRepo, bus)
	notifSvc := notifservice.NewNotificationService(deviceRepo, fcmSender)
	gmailSvc := gmailservice.NewGmailService(
		gmailservice.Config{
			ClientID:        cfg.Google.ClientID,
			ClientSecret:    cfg.Google.ClientSecret,
			RedirectURL:     cfg.Google.GmailRedirectURL,
			HMACSecret:      cfg.JWT.AccessSecret,
			MapsAPIKey:      cfg.Google.MapsAPIKey,
			AnthropicAPIKey: cfg.Anthropic.APIKey,
			LLMModel:        cfg.Anthropic.Model,
		},
		gmailConnRepo,
		gmailLogRepo,
		gmailProvRepo,
		activitySvc,
	)

	// --- Event subscriptions ---
	bus.Subscribe(actdomain.EventActivityIngested, emissionSvc.HandleActivityIngested)
	bus.Subscribe(emidomain.EventEmissionCalculated, timelineSvc.HandleEmissionCalculated)
	bus.Subscribe(emidomain.EventEmissionCalculated, insightSvc.HandleEmissionCalculated)
	bus.Subscribe(insightdomain.EventInsightCreated, notifSvc.HandleInsightCreated)
	bus.Subscribe(actdomain.EventActivityPossibleDuplicate, notifSvc.HandleActivityPossibleDuplicate)

	// --- Cron scheduler ---
	c := cron.New()

	// Gmail sync — every 6 hours
	c.AddFunc("0 */6 * * *", func() {
		log.Info("cron: starting gmail sync")
		result := gmailSvc.SyncAll(context.Background())
		log.Info("cron: gmail sync complete",
			zap.Int("total", result.Total),
			zap.Int("synced", result.Synced),
			zap.Int("skipped", result.Skipped),
			zap.Int("failed", result.Failed),
		)
	})

	// LLM enrichment for unrecognised emails — every 12 hours, offset from sync
	c.AddFunc("0 1,13 * * *", func() {
		log.Info("cron: starting llm enrichment")
		result := gmailSvc.EnrichUnrecognisedAll(context.Background())
		log.Info("cron: llm enrichment complete",
			zap.Int("total_users", result.TotalUsers),
			zap.Int("enriched", result.Enriched),
			zap.Int("failed", result.Failed),
		)
	})

	// Purge soft-deleted accounts — Monday and Thursday at 03:00
	c.AddFunc("0 3 * * 1,4", func() {
		log.Info("cron: starting account purge")
		n, err := identitySvc.PurgeDeletedAccounts(context.Background())
		if err != nil {
			log.Error("cron: account purge failed", zap.Error(err))
			return
		}
		log.Info("cron: account purge complete", zap.Int64("purged", n))
	})

	c.Start()
	log.Info("worker started",
		zap.String("env", cfg.App.Env),
		zap.Int("jobs", len(c.Entries())),
	)

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("worker shutting down")
	ctx := c.Stop()
	<-ctx.Done()
	log.Info("worker stopped")
}
