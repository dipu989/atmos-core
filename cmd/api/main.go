// @title          Atmos API
// @version        1.0
// @description    Personal environmental telemetry and carbon intelligence platform.
// @termsOfService http://swagger.io/terms/

// @contact.name  Atmos Team
// @contact.email me.shantnu@gmail.com

// @license.name MIT

// @host     localhost:8081
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in                         header
// @name                       Authorization
// @description                Enter: Bearer <your_access_token>
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/dipu/atmos-core/docs"

	"github.com/dipu/atmos-core/config"
	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
	acthandler "github.com/dipu/atmos-core/internal/activity/handler"
	actrepo "github.com/dipu/atmos-core/internal/activity/repository"
	actservice "github.com/dipu/atmos-core/internal/activity/service"
	authhandler "github.com/dipu/atmos-core/internal/auth/handler"
	authrepo "github.com/dipu/atmos-core/internal/auth/repository"
	authservice "github.com/dipu/atmos-core/internal/auth/service"
	devhandler "github.com/dipu/atmos-core/internal/device/handler"
	devrepo "github.com/dipu/atmos-core/internal/device/repository"
	devservice "github.com/dipu/atmos-core/internal/device/service"
	emidomain "github.com/dipu/atmos-core/internal/emission/domain"
	emirepo "github.com/dipu/atmos-core/internal/emission/repository"
	emiservice "github.com/dipu/atmos-core/internal/emission/service"
	gmailhandler "github.com/dipu/atmos-core/internal/gmail/handler"
	gmailrepo "github.com/dipu/atmos-core/internal/gmail/repository"
	gmailservice "github.com/dipu/atmos-core/internal/gmail/service"
	idhandler "github.com/dipu/atmos-core/internal/identity/handler"
	idrepo "github.com/dipu/atmos-core/internal/identity/repository"
	idservice "github.com/dipu/atmos-core/internal/identity/service"
	insightdomain "github.com/dipu/atmos-core/internal/insight/domain"
	insighthandler "github.com/dipu/atmos-core/internal/insight/handler"
	insightrepo "github.com/dipu/atmos-core/internal/insight/repository"
	insightservice "github.com/dipu/atmos-core/internal/insight/service"
	notifservice "github.com/dipu/atmos-core/internal/notification/service"
	placeshandler "github.com/dipu/atmos-core/internal/places"
	timelineagg "github.com/dipu/atmos-core/internal/timeline/aggregator"
	timelinehandler "github.com/dipu/atmos-core/internal/timeline/handler"
	timelinerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	timelineservice "github.com/dipu/atmos-core/internal/timeline/service"
	"github.com/dipu/atmos-core/platform/database"
	"github.com/dipu/atmos-core/platform/email"
	"github.com/dipu/atmos-core/platform/eventbus"
	jwtpkg "github.com/dipu/atmos-core/platform/jwt"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/push"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	fiberSwagger "github.com/swaggo/fiber-swagger"
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

	// --- Email sender ---
	emailSender, err := email.NewSESSender(context.Background(), cfg.Email.Region, cfg.Email.FromAddr)
	if err != nil {
		log.Fatal("SES client init failed", zap.Error(err))
	}

	// --- FCM push sender (nil when not configured) ---
	var fcmSender push.Sender
	if cfg.Push.FCMServiceAccountJSON != "" {
		fcmSender, err = push.NewFCMSender(context.Background(), cfg.Push.FCMServiceAccountJSON)
		if err != nil {
			log.Fatal("FCM client init failed", zap.Error(err))
		}
		log.Info("FCM push notifications enabled")
	} else {
		log.Info("FCM push notifications disabled — set FCM_SERVICE_ACCOUNT_JSON to enable")
	}

	// --- Repositories ---
	userRepo := idrepo.NewUserRepository(db)
	tokenRepo := authrepo.NewTokenRepository(db)
	resetRepo := authrepo.NewPasswordResetRepository(db)
	verificationRepo := authrepo.NewEmailVerificationRepository(db)
	deviceRepo := devrepo.NewDeviceRepository(db)
	activityRepo := actrepo.NewActivityRepository(db)
	emissionRepo := emirepo.NewEmissionRepository(db)
	summaryRepo := timelinerepo.NewSummaryRepository(db)
	insightRepo := insightrepo.NewInsightRepository(db)
	gmailConnRepo := gmailrepo.NewConnectionRepository(db)
	gmailLogRepo := gmailrepo.NewLogRepository(db)
	gmailProvRepo := gmailrepo.NewProviderRepository(db)

	// --- JWT ---
	jwtManager := jwtpkg.NewManager(
		cfg.JWT.AccessSecret,
		cfg.JWT.RefreshSecret,
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
	)

	// --- Services ---
	authSvc := authservice.NewAuthService(
		userRepo, tokenRepo, resetRepo, verificationRepo, jwtManager,
		authservice.Config{
			GoogleClientID:     cfg.Google.ClientID,
			GoogleIosClientID:  cfg.Google.IosClientID,
			GoogleClientSecret: cfg.Google.ClientSecret,
			GoogleRedirectURL:  cfg.Google.RedirectURL,
			EmailSender:        emailSender,
			FrontendURL:        cfg.App.FrontendURL,
		},
	)
	identitySvc := idservice.NewIdentityService(userRepo, tokenRepo)
	deviceSvc := devservice.NewDeviceService(deviceRepo)
	activitySvc := actservice.NewActivityService(activityRepo, bus)
	emissionSvc := emiservice.NewEmissionService(emissionRepo, activityRepo, bus)
	agg := timelineagg.NewAggregator(summaryRepo)
	timelineSvc := timelineservice.NewTimelineService(summaryRepo, agg)
	insightSvc := insightservice.NewInsightService(insightRepo, summaryRepo, bus)
	notifSvc := notifservice.NewNotificationService(deviceRepo, fcmSender)
	gmailSvc := gmailservice.NewGmailService(
		gmailservice.Config{
			ClientID:     cfg.Google.ClientID,
			ClientSecret: cfg.Google.ClientSecret,
			RedirectURL:  cfg.Google.GmailRedirectURL,
			HMACSecret:   cfg.JWT.AccessSecret,
			MapsAPIKey:   cfg.Google.MapsAPIKey,
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

	// --- Handlers ---
	authH := authhandler.NewAuthHandler(authSvc, cfg.App.FrontendURL)
	identityH := idhandler.NewIdentityHandler(identitySvc)
	deviceH := devhandler.NewDeviceHandler(deviceSvc)
	activityH := acthandler.NewActivityHandler(activitySvc)
	timelineH := timelinehandler.NewTimelineHandler(timelineSvc)
	insightH := insighthandler.NewInsightHandler(insightSvc)
	gmailH := gmailhandler.NewGmailHandler(gmailSvc)
	providerH := gmailhandler.NewProviderHandler(gmailProvRepo)
	placesH := placeshandler.NewHandler(cfg.Google.MapsAPIKey)

	// --- Fiber app ---
	app := fiber.New(fiber.Config{
		AppName:               "Atmos API",
		ErrorHandler:          globalErrorHandler,
		DisableStartupMessage: cfg.App.Env == "production",
	})

	// --- Global middleware ---
	app.Use(recover.New())
	app.Use(middleware.RequestID())
	app.Use(middleware.CORS(cfg.App.CORSAllowOrigin))
	app.Use(middleware.RateLimit())

	// --- Health + Swagger (unauthenticated) ---
	app.Get("/health", healthHandler)
	app.Get("/swagger/*", fiberSwagger.WrapHandler)

	// --- API v1 ---
	api := app.Group("/api/v1")

	// Public provider catalogue — no auth required
	api.Get("/providers", providerH.ListActive)
	api.Get("/providers/all", providerH.ListAll)

	// Gmail OAuth callback — unauthenticated (Google redirects here with ?code=&state=)
	// Must be registered before api.Use(RequireAuth) or Fiber will apply auth middleware to it.
	api.Get("/gmail/callback", gmailH.Callback)

	// Auth (stricter rate limit on these endpoints)
	auth := api.Group("/auth", middleware.RateLimitStrict())
	auth.Post("/register", authH.Register)
	auth.Post("/login", authH.Login)
	auth.Post("/logout", authH.Logout)
	auth.Post("/token/refresh", authH.Refresh)
	auth.Post("/google/token", authH.GoogleTokenLogin) // mobile: ID token from native SDK
	auth.Get("/google/login", authH.GoogleLogin)
	auth.Get("/google/callback", authH.GoogleCallback)
	auth.Post("/verify-email", authH.VerifyEmail)
	auth.Post("/forgot-password", authH.ForgotPassword)
	auth.Post("/reset-password", authH.ResetPassword)

	// Protected
	protected := api.Use(middleware.RequireAuth(jwtManager))

	protected.Post("/auth/resend-verification", authH.ResendVerification)

	protected.Get("/users/me", identityH.GetMe)
	protected.Put("/users/me", identityH.UpdateMe)
	protected.Delete("/users/me", identityH.DeleteAccount)
	protected.Get("/users/me/preferences", identityH.GetPreferences)
	protected.Put("/users/me/preferences", identityH.UpdatePreferences)

	protected.Post("/devices/register", deviceH.Register)
	protected.Get("/devices", deviceH.List)
	protected.Patch("/devices/:id", deviceH.UpdatePushToken)
	protected.Delete("/devices/:id", deviceH.Deregister)

	protected.Get("/places/autocomplete", placesH.Autocomplete)

	protected.Post("/activities", activityH.Ingest)
	protected.Get("/activities", activityH.ListActivities)
	protected.Get("/activities/:id", activityH.GetActivity)
	protected.Patch("/activities/:id", activityH.UpdateActivity)
	protected.Delete("/activities/:id", activityH.DeleteActivity)

	protected.Get("/timeline/daily", timelineH.GetDaily)
	protected.Get("/timeline/weekly", timelineH.GetWeekly)
	protected.Get("/timeline/monthly", timelineH.GetMonthly)
	protected.Get("/timeline/day/:date", timelineH.GetDay)
	protected.Get("/timeline/week/:week_start", timelineH.GetWeek)
	protected.Get("/timeline/month/:year/:month", timelineH.GetMonth)
	protected.Get("/timeline/range", timelineH.GetRange)

	protected.Get("/insights", insightH.ListInsights)
	protected.Get("/insights/:id", insightH.GetInsight)
	protected.Patch("/insights/:id/read", insightH.MarkRead)

	// Gmail email ingestion
	// /gmail/connect  → starts OAuth (requires auth so we know the user)
	// /gmail/callback → unauthenticated (Google redirects here with ?code=&state=)
	protected.Get("/gmail/connect", gmailH.Connect)
	protected.Get("/gmail/auth-url", gmailH.AuthURL)
	protected.Get("/gmail/status", gmailH.Status)
	protected.Delete("/gmail/disconnect", gmailH.Disconnect)
	protected.Post("/gmail/sync", gmailH.Sync)
	protected.Get("/gmail/logs", gmailH.Logs)

	// --- Internal endpoints (cron-triggered, not user-facing) ---
	// Called by Linux cron: POST /internal/gmail/sync-all
	// Protected by X-Internal-Key header (INTERNAL_SYNC_KEY env var).
	internal := app.Group("/internal", middleware.RequireInternalKey(cfg.App.InternalSyncKey))
	internal.Post("/gmail/sync-all", gmailH.SyncAll)
	internal.Post("/users/purge-deleted", func(c *fiber.Ctx) error {
		n, err := identitySvc.PurgeDeletedAccounts(c.Context())
		if err != nil {
			return response.InternalError(c, "purge failed: "+err.Error())
		}
		return response.OK(c, fiber.Map{"purged": n})
	})

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Info("shutting down server")
		_ = app.Shutdown()
	}()

	log.Info("starting server",
		zap.String("port", cfg.App.Port),
		zap.String("env", cfg.App.Env),
		zap.String("swagger", "http://localhost:"+cfg.App.Port+"/swagger/index.html"),
	)
	if err := app.Listen(":" + cfg.App.Port); err != nil {
		log.Fatal("server error", zap.Error(err))
	}
}

// healthHandler godoc
// @Summary     Health check
// @Description Returns server and dependency status
// @Tags        system
// @Produce     json
// @Success     200 {object} map[string]string
// @Router      /health [get]
func healthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok", "version": "1.0"})
}

func globalErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error":   err.Error(),
	})
}
