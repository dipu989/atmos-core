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
	idhandler "github.com/dipu/atmos-core/internal/identity/handler"
	idrepo "github.com/dipu/atmos-core/internal/identity/repository"
	idservice "github.com/dipu/atmos-core/internal/identity/service"
	insighthandler "github.com/dipu/atmos-core/internal/insight/handler"
	insightrepo "github.com/dipu/atmos-core/internal/insight/repository"
	insightservice "github.com/dipu/atmos-core/internal/insight/service"
	timelineagg "github.com/dipu/atmos-core/internal/timeline/aggregator"
	timelinehandler "github.com/dipu/atmos-core/internal/timeline/handler"
	timelinerepo "github.com/dipu/atmos-core/internal/timeline/repository"
	timelineservice "github.com/dipu/atmos-core/internal/timeline/service"
	"github.com/dipu/atmos-core/platform/database"
	"github.com/dipu/atmos-core/platform/eventbus"
	jwtpkg "github.com/dipu/atmos-core/platform/jwt"
	"github.com/dipu/atmos-core/platform/logger"
	"github.com/dipu/atmos-core/platform/middleware"
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

	// --- Repositories ---
	userRepo     := idrepo.NewUserRepository(db)
	tokenRepo    := authrepo.NewTokenRepository(db)
	deviceRepo   := devrepo.NewDeviceRepository(db)
	activityRepo := actrepo.NewActivityRepository(db)
	emissionRepo := emirepo.NewEmissionRepository(db)
	summaryRepo  := timelinerepo.NewSummaryRepository(db)
	insightRepo  := insightrepo.NewInsightRepository(db)

	// --- JWT ---
	jwtManager := jwtpkg.NewManager(
		cfg.JWT.AccessSecret,
		cfg.JWT.RefreshSecret,
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
	)

	// --- Services ---
	authSvc     := authservice.NewAuthService(userRepo, tokenRepo, jwtManager)
	identitySvc := idservice.NewIdentityService(userRepo)
	deviceSvc   := devservice.NewDeviceService(deviceRepo)
	activitySvc := actservice.NewActivityService(activityRepo, bus)
	emissionSvc := emiservice.NewEmissionService(emissionRepo, activityRepo, bus)
	agg         := timelineagg.NewAggregator(summaryRepo)
	timelineSvc := timelineservice.NewTimelineService(summaryRepo, agg)
	insightSvc  := insightservice.NewInsightService(insightRepo)

	// --- Event subscriptions ---
	bus.Subscribe(actdomain.EventActivityIngested, emissionSvc.HandleActivityIngested)
	bus.Subscribe(emidomain.EventEmissionCalculated, timelineSvc.HandleEmissionCalculated)
	bus.Subscribe(emidomain.EventEmissionCalculated, insightSvc.HandleEmissionCalculated)

	// --- Handlers ---
	authH     := authhandler.NewAuthHandler(authSvc)
	identityH := idhandler.NewIdentityHandler(identitySvc)
	deviceH   := devhandler.NewDeviceHandler(deviceSvc)
	activityH := acthandler.NewActivityHandler(activitySvc)
	timelineH := timelinehandler.NewTimelineHandler(timelineSvc)
	insightH  := insighthandler.NewInsightHandler(insightSvc)

	// --- Fiber app ---
	app := fiber.New(fiber.Config{
		AppName:               "Atmos API",
		ErrorHandler:          globalErrorHandler,
		DisableStartupMessage: cfg.App.Env == "production",
	})

	// --- Global middleware ---
	app.Use(recover.New())
	app.Use(middleware.RequestID())
	app.Use(middleware.CORS())
	app.Use(middleware.RateLimit())

	// --- Health + Swagger (unauthenticated) ---
	app.Get("/health", healthHandler)
	app.Get("/swagger/*", fiberSwagger.WrapHandler)

	// --- API v1 ---
	api := app.Group("/api/v1")

	// Auth (stricter rate limit on these endpoints)
	auth := api.Group("/auth", middleware.RateLimitStrict())
	auth.Post("/register",      authH.Register)
	auth.Post("/login",         authH.Login)
	auth.Post("/logout",        authH.Logout)
	auth.Post("/token/refresh", authH.Refresh)

	// Protected
	protected := api.Use(middleware.RequireAuth(jwtManager))

	protected.Get("/users/me",   identityH.GetMe)
	protected.Patch("/users/me", identityH.UpdateMe)

	protected.Post("/devices",       deviceH.Register)
	protected.Get("/devices",        deviceH.List)
	protected.Patch("/devices/:id",  deviceH.UpdatePushToken)
	protected.Delete("/devices/:id", deviceH.Deregister)

	protected.Post("/activities",    activityH.Ingest)
	protected.Get("/activities",     activityH.ListActivities)
	protected.Get("/activities/:id", activityH.GetActivity)

	protected.Get("/timeline/day/:date",         timelineH.GetDay)
	protected.Get("/timeline/week/:week_start",   timelineH.GetWeek)
	protected.Get("/timeline/month/:year/:month", timelineH.GetMonth)
	protected.Get("/timeline/range",              timelineH.GetRange)

	protected.Get("/insights",           insightH.ListInsights)
	protected.Get("/insights/:id",       insightH.GetInsight)
	protected.Patch("/insights/:id/read", insightH.MarkRead)

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
