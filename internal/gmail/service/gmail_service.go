// Package service orchestrates the Gmail OAuth lifecycle and email ingestion pipeline.
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"

	actdomain "github.com/dipu/atmos-core/internal/activity/domain"
	actservice "github.com/dipu/atmos-core/internal/activity/service"
	"github.com/dipu/atmos-core/internal/geocoder"
	"github.com/dipu/atmos-core/internal/gmail/domain"
	"github.com/dipu/atmos-core/internal/gmail/parser"
	"github.com/dipu/atmos-core/internal/gmail/repository"
	"github.com/dipu/atmos-core/platform/logger"
	uuidpkg "github.com/dipu/atmos-core/platform/uuid"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

var ErrNotConnected = errors.New("gmail not connected for this user")

// firstSyncLookback: how far back on the very first sync (no historyId yet).
const firstSyncLookback = "newer_than:90d"

// historyFallbackWindow: used when the stored historyId has expired.
// Gmail history records expire after ~7 days of inactivity.
const historyFallbackWindow = "newer_than:7d"

// defaultBatchSize: messages fetched per sync call (quota: ~500 units / sync).
const defaultBatchSize int64 = 100

// workerCount: concurrent goroutines for message processing.
// Gmail's per-user rate limit is 250 quota units/second; 5 workers is safe.
const workerCount = 5

// syncCooldown: minimum gap between two syncs for the same user.
// The internal /sync-all endpoint respects this; manual /sync does not.
const syncCooldown = 23 * time.Hour

// ── Service ──────────────────────────────────────────────────────────────────

type GmailService struct {
	oauthCfg    *oauth2.Config
	connRepo    *repository.ConnectionRepository
	logRepo     *repository.LogRepository
	provRepo    *repository.ProviderRepository
	activitySvc *actservice.ActivityService
	registry    *parser.Registry
	geocoder    geocoder.Geocoder
	llmParser   *parser.LLMParser
	hmacSecret  []byte
	batchSize   int64
}

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	HMACSecret   string // JWT_ACCESS_SECRET is reused
	BatchSize    int64  // 0 → defaultBatchSize
	MapsAPIKey   string // optional; enables geocoding of pickup/drop addresses
	GroqAPIKey   string // GROQ_API_KEY — enables LLM enrichment (empty disables it)
	GroqModel    string // GROQ_MODEL — model to use (default: llama-3.1-8b-instant)
}

func NewGmailService(
	cfg Config,
	connRepo *repository.ConnectionRepository,
	logRepo *repository.LogRepository,
	provRepo *repository.ProviderRepository,
	activitySvc *actservice.ActivityService,
) *GmailService {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	return &GmailService{
		oauthCfg: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{googleapi.GmailReadonlyScope},
			Endpoint:     google.Endpoint,
		},
		connRepo:    connRepo,
		logRepo:     logRepo,
		provRepo:    provRepo,
		activitySvc: activitySvc,
		registry:    parser.NewRegistry(),
		geocoder:    geocoder.New(cfg.MapsAPIKey),
		llmParser:   parser.NewLLMParser(cfg.GroqAPIKey, cfg.GroqModel, 1),
		hmacSecret:  []byte(cfg.HMACSecret),
		batchSize:   batchSize,
	}
}

// ── OAuth connect / disconnect ────────────────────────────────────────────────

// AuthURL generates the Google consent-page URL for web flows.
// Returns an empty string if Google OAuth is not configured (missing ClientID).
// The userID is embedded in a signed state parameter — no session cookie needed.
func (s *GmailService) AuthURL(userID uuid.UUID) string {
	return s.AuthURLForPlatform(userID, "")
}

// AuthURLForPlatform generates the consent-page URL, embedding [platform] in the
// signed state so the callback can redirect back appropriately.
// Use platform="mobile" for in-app browser flows; leave empty for web.
func (s *GmailService) AuthURLForPlatform(userID uuid.UUID, platform string) string {
	if s.oauthCfg.ClientID == "" {
		return ""
	}
	return s.oauthCfg.AuthCodeURL(
		s.signState(userID.String(), platform),
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)
}

// HandleCallback exchanges the OAuth code for tokens and persists the connection.
// Returns the connection and the originating platform ("mobile" or "") so the
// caller can choose the appropriate post-auth redirect destination.
func (s *GmailService) HandleCallback(ctx context.Context, state, code string) (*domain.GmailConnection, string, error) {
	userIDStr, platform, err := s.verifyState(state)
	if err != nil {
		return nil, "", fmt.Errorf("invalid oauth state: %w", err)
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, platform, fmt.Errorf("invalid user id in state: %w", err)
	}
	token, err := s.oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, platform, fmt.Errorf("token exchange: %w", err)
	}
	email, err := s.fetchGmailEmail(ctx, token)
	if err != nil {
		return nil, platform, fmt.Errorf("fetch gmail profile: %w", err)
	}
	now := time.Now().UTC()
	conn := &domain.GmailConnection{
		ID:           uuidpkg.New(),
		UserID:       userID,
		Email:        email,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenExpiry:  token.Expiry.UTC(),
		ConnectedAt:  now,
		UpdatedAt:    now,
	}
	if err := s.connRepo.Upsert(ctx, conn); err != nil {
		return nil, platform, fmt.Errorf("save gmail connection: %w", err)
	}
	return conn, platform, nil
}

// Disconnect removes the stored Gmail tokens for a user.
func (s *GmailService) Disconnect(ctx context.Context, userID uuid.UUID) error {
	return s.connRepo.DeleteByUserID(ctx, userID)
}

// Status returns the connection (including last_sync_summary) or nil.
func (s *GmailService) Status(ctx context.Context, userID uuid.UUID) (*domain.GmailConnection, error) {
	return s.connRepo.FindByUserID(ctx, userID)
}

// ── Sync ─────────────────────────────────────────────────────────────────────

// SyncResult summarises one sync run.
type SyncResult struct {
	MessagesChecked int `json:"messages_checked"`
	Parsed          int `json:"parsed"`
	Skipped         int `json:"skipped"`
	Failed          int `json:"failed"`
	Unrecognised    int `json:"unrecognised,omitempty"` // queued for LLM enrichment
}

// Sync fetches ride-receipt emails from Gmail and ingests them as activities.
//
// Fetch strategy (incremental by default):
//   - First sync (no historyId)   → query  = senders + newer_than:90d
//   - Subsequent syncs            → History API from stored historyId (zero re-scan)
//   - historyId expired (>7 days) → query  = senders + newer_than:7d
//
// Deduplication (two independent layers):
//   - email_ingestion_logs UNIQUE (user_id, message_id): checked before body fetch
//   - activities.idempotency_key = "gmail:<messageId>":  DB-level backstop
//
// Processing: up to workerCount (5) goroutines in parallel.
// Batch size: s.batchSize messages per call (default 100).
func (s *GmailService) Sync(ctx context.Context, userID uuid.UUID) (*SyncResult, error) {
	conn, err := s.connRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, ErrNotConnected
	}

	// Build routing map and Gmail query from the DB — no hardcoded senders.
	routingMap, err := s.provRepo.BuildRoutingMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("build routing map: %w", err)
	}
	gmailQuery, err := s.provRepo.GmailQuery(ctx)
	if err != nil {
		return nil, fmt.Errorf("build gmail query: %w", err)
	}
	if gmailQuery == "" {
		return &SyncResult{}, nil // no active providers yet
	}

	gmailSvc, err := s.gmailClient(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("gmail client: %w", err)
	}

	messageIDs, newHistoryID, err := s.listMessageIDs(ctx, gmailSvc, conn, gmailQuery)
	if err != nil {
		return nil, err
	}

	result := s.processMessages(ctx, gmailSvc, userID, messageIDs, routingMap)

	// Persist updated cursor + summary so the next sync is incremental
	// and GET /gmail/status can return last_sync_summary without extra DB query.
	now := time.Now().UTC()
	summary := domain.SyncSummary(*result)
	conn.LastSyncAt = &now
	conn.LastSyncSummary = &summary
	conn.UpdatedAt = now
	if newHistoryID != "" {
		conn.HistoryID = &newHistoryID
	}
	if err := s.connRepo.Save(ctx, conn); err != nil {
		logger.L().Warn("gmail: failed to persist sync state", zap.Error(err))
	}

	return result, nil
}

// SyncAll triggers Sync for every connected user whose last sync was more than
// syncCooldown (23 h) ago. Called by POST /internal/gmail/sync-all (Linux cron).
// Errors per user are logged but do not abort others.
func (s *GmailService) SyncAll(ctx context.Context) *SyncAllResult {
	log := logger.L()

	conns, err := s.connRepo.FindAllConnected(ctx)
	if err != nil {
		log.Error("gmail SyncAll: list connections", zap.Error(err))
		return &SyncAllResult{Error: err.Error()}
	}

	res := &SyncAllResult{Total: len(conns)}
	for _, conn := range conns {
		// Cooldown guard: the real "once per day" guarantee.
		// Even if the cron fires twice or multiple containers run SyncAll
		// simultaneously, a user is never synced more than once per 23 h.
		if conn.LastSyncAt != nil && time.Since(*conn.LastSyncAt) < syncCooldown {
			res.Skipped++
			continue
		}

		if _, err := s.Sync(ctx, conn.UserID); err != nil {
			log.Warn("gmail SyncAll: user sync failed",
				zap.Stringer("user_id", conn.UserID),
				zap.Error(err),
			)
			res.Failed++
			continue
		}
		res.Synced++
	}

	log.Info("gmail SyncAll: complete",
		zap.Int("total", res.Total),
		zap.Int("synced", res.Synced),
		zap.Int("skipped", res.Skipped),
		zap.Int("failed", res.Failed),
	)
	return res
}

type SyncAllResult struct {
	Total   int    `json:"total"`
	Synced  int    `json:"synced"`
	Skipped int    `json:"skipped"`
	Failed  int    `json:"failed"`
	Error   string `json:"error,omitempty"`
}

// Logs returns paginated ingestion history for a user.
func (s *GmailService) Logs(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.EmailIngestionLog, int64, error) {
	return s.logRepo.List(ctx, userID, limit, offset)
}

// ResetSync clears the stored historyId for a user's Gmail connection so the
// next Sync call performs a full re-scan (up to firstSyncLookback days back)
// rather than an incremental History API call. Useful after a parser fix to
// reprocess existing emails and backfill missing fields.
func (s *GmailService) ResetSync(ctx context.Context, userID uuid.UUID) error {
	conn, err := s.connRepo.FindByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if conn == nil {
		return ErrNotConnected
	}
	conn.HistoryID = nil
	return s.connRepo.Save(ctx, conn)
}

// ── Message listing ───────────────────────────────────────────────────────────

func (s *GmailService) listMessageIDs(
	ctx context.Context,
	svc *googleapi.Service,
	conn *domain.GmailConnection,
	baseQuery string,
) (ids []string, newHistoryID string, err error) {

	// Incremental path via History API when we have a stored historyId.
	if conn.HistoryID != nil && *conn.HistoryID != "" {
		ids, newHistoryID, err = s.listViaHistory(ctx, svc, *conn.HistoryID)
		if err == nil {
			return
		}
		// History ID expired — fall through to time-window query.
		logger.L().Warn("gmail: historyId expired, falling back to window query",
			zap.Error(err))
	}

	// First sync or fallback: time-window query.
	window := firstSyncLookback
	if conn.HistoryID != nil {
		window = historyFallbackWindow
	}
	ids, newHistoryID, err = s.listViaQuery(ctx, svc, baseQuery+" "+window)
	return
}

func (s *GmailService) listViaQuery(ctx context.Context, svc *googleapi.Service, query string) ([]string, string, error) {
	resp, err := svc.Users.Messages.List("me").
		Q(query).
		MaxResults(s.batchSize).
		Context(ctx).
		Do()
	if err != nil {
		return nil, "", fmt.Errorf("gmail list: %w", err)
	}

	ids := make([]string, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		ids = append(ids, m.Id)
	}

	// Capture historyId from the most recent message for the next incremental sync.
	var historyID string
	if len(resp.Messages) > 0 {
		if msg, err := svc.Users.Messages.Get("me", resp.Messages[0].Id).
			Format("metadata").Context(ctx).Do(); err == nil {
			historyID = fmt.Sprintf("%d", msg.HistoryId)
		}
	}
	return ids, historyID, nil
}

func (s *GmailService) listViaHistory(ctx context.Context, svc *googleapi.Service, historyID string) ([]string, string, error) {
	var startID uint64
	if _, err := fmt.Sscan(historyID, &startID); err != nil {
		return nil, "", fmt.Errorf("invalid historyId %q: %w", historyID, err)
	}

	resp, err := svc.Users.History.List("me").
		StartHistoryId(startID).
		HistoryTypes("messageAdded").
		LabelId("INBOX").
		MaxResults(s.batchSize).
		Context(ctx).
		Do()
	if err != nil {
		return nil, "", fmt.Errorf("gmail history list: %w", err)
	}

	seen := make(map[string]bool)
	var ids []string
	for _, h := range resp.History {
		for _, ma := range h.MessagesAdded {
			if !seen[ma.Message.Id] {
				seen[ma.Message.Id] = true
				ids = append(ids, ma.Message.Id)
			}
		}
	}
	return ids, fmt.Sprintf("%d", resp.HistoryId), nil
}

// ── Concurrent message processing ─────────────────────────────────────────────

type msgJob struct {
	messageID string
}

type msgOutcome struct {
	messageID string
	log       *domain.EmailIngestionLog
	counted   bool // false for already-processed (skipped before any API call)
}

func (s *GmailService) processMessages(
	ctx context.Context,
	gmailSvc *googleapi.Service,
	userID uuid.UUID,
	messageIDs []string,
	routingMap repository.RoutingMap,
) *SyncResult {
	log := logger.L()
	result := &SyncResult{MessagesChecked: len(messageIDs)}

	jobs := make(chan msgJob, len(messageIDs))
	outcomes := make(chan msgOutcome, len(messageIDs))

	// Spawn workers.
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				outcome := s.handleMessage(ctx, gmailSvc, userID, job.messageID, routingMap)
				outcomes <- outcome
			}
		}()
	}

	// Feed jobs.
	for _, id := range messageIDs {
		jobs <- msgJob{id}
	}
	close(jobs)

	// Close outcomes once all workers finish.
	go func() {
		wg.Wait()
		close(outcomes)
	}()

	// Collect outcomes.
	for o := range outcomes {
		if o.log != nil {
			if err := s.logRepo.Create(ctx, o.log); err != nil {
				log.Warn("gmail: save log entry", zap.Error(err), zap.String("msg_id", o.messageID))
			}
			switch o.log.Status {
			case domain.StatusParsed:
				result.Parsed++
			case domain.StatusFailed:
				result.Failed++
			case domain.StatusUnrecognised:
				result.Unrecognised++
			default:
				result.Skipped++
			}
		} else {
			// Already processed — counted as skipped, no log write needed.
			result.Skipped++
		}
	}
	return result
}

// handleMessage processes a single Gmail message.
// Returns an outcome with a log entry to persist, or nil log if already processed.
func (s *GmailService) handleMessage(
	ctx context.Context,
	gmailSvc *googleapi.Service,
	userID uuid.UUID,
	messageID string,
	routingMap repository.RoutingMap,
) msgOutcome {
	// ── Layer 1 dedup: skip if already in email_ingestion_logs ───────────────
	existingLog, err := s.logRepo.FindByMessageID(ctx, userID, messageID)
	if err != nil {
		// Treat a DB error as "already seen" to avoid duplicate processing and
		// the unique-constraint violation that would follow on logRepo.Create.
		logger.L().Warn("gmail: idempotency check failed, skipping message", zap.Error(err))
		return msgOutcome{messageID: messageID}
	}
	if existingLog != nil {
		// Already processed — opportunistically backfill origin/destination only
		// when the activity is still missing them. The HasRouteLabels check avoids
		// a Gmail API fetch for the common case where addresses are already set.
		if existingLog.ActivityID != nil {
			has, err := s.activitySvc.HasRouteLabels(ctx, *existingLog.ActivityID)
			if err != nil {
				logger.L().Warn("gmail: HasRouteLabels check failed, skipping backfill",
					zap.Stringer("activity_id", *existingLog.ActivityID), zap.Error(err))
			} else if !has {
				s.tryBackfillAddresses(ctx, gmailSvc, userID, messageID, *existingLog.ActivityID, routingMap)
			}
		}
		return msgOutcome{messageID: messageID} // nil log = already done
	}

	// ── Fetch full message (headers + snippet + body in one call) ────────────
	msg, err := gmailSvc.Users.Messages.Get("me", messageID).
		Format("full").
		Context(ctx).
		Do()
	if err != nil {
		return s.failOutcome(userID, messageID, nil, "", "", fmt.Errorf("get message: %w", err))
	}

	fromRaw, subject, dateHeader := extractHeaders(msg)
	snippet := msg.Snippet

	// ── Cancellation guard (cheap — no parser needed) ────────────────────────
	if parser.IsCancellation(subject, snippet) {
		return s.skipOutcome(userID, messageID, nil, subject, snippet)
	}

	// ── Route: sender_email → candidate ProviderEmailTypes ───────────────────
	// extractEmailAddress handles "Display Name <addr@host>" and bare "addr@host".
	from := extractEmailAddress(fromRaw)
	candidates := routingMap[strings.ToLower(from)]
	if len(candidates) == 0 {
		// Sender not in active providers — silently skip (no log entry).
		return msgOutcome{messageID: messageID}
	}

	// Filter candidates by subject_pattern when set.
	candidates = filterBySubject(candidates, subject)
	if len(candidates) == 0 {
		return s.skipOutcome(userID, messageID, nil, subject, snippet)
	}

	// ── Full-body decode (done once, shared by both paths) ───────────────────
	// Extract raw HTML before stripping so we can recover map coords from <a href>
	// attributes — they are lost after plain-text conversion.
	rawHTML := extractPart(msg.Payload, "text/html")
	htmlPickupLat, htmlPickupLng, htmlDropLat, htmlDropLng := parser.ExtractMapCoords(rawHTML)
	body := extractBody(msg)

	// ── LLM-first path ───────────────────────────────────────────────────────
	// When Groq is configured, try it before the regex parsers. The LLM can
	// extract pickup/drop addresses that regex cannot, and handles format
	// changes in provider emails automatically.
	if s.llmParser.IsEnabled() {
		// Use plain text only — raw HTML balloons token usage and hits TPM limits.
		// body is already text/plain, falling back to stripped HTML when unavailable.
		ride, err := s.llmParser.ParseWithContext(ctx, subject, body)
		if err == nil {
			if ride.PickupLat == nil {
				ride.PickupLat, ride.PickupLng = htmlPickupLat, htmlPickupLng
			}
			if ride.DropLat == nil {
				ride.DropLat, ride.DropLng = htmlDropLat, htmlDropLng
			}
			resolved := resolveCode(candidates, ride.ProviderEmailTypeCode, candidates[0].Code)
			return s.ingestRide(ctx, userID, messageID, resolved, candidates[0], dateHeader, subject, snippet, ride)
		}
		if errors.Is(err, parser.ErrUnrecognisedFormat) {
			return s.skipOutcome(userID, messageID, &candidates[0].Code, subject, snippet)
		}
		// Transient error (network, rate-limit) — fall through to regex parsers.
		logger.L().Warn("gmail sync: llm parse failed, falling back to regex",
			zap.String("msg_id", messageID), zap.Error(err))
	}

	for _, candidate := range candidates {
		p, ok := s.registry.Get(candidate.Code)
		if !ok {
			continue
		}

		// Try the snippet first — it is cheap and sufficient for distance/duration.
		// If it succeeds, still attempt a full-body parse so we can enrich the ride
		// with pickup/drop addresses and HTML-embedded map coordinates.
		ride, ok := p.TrySnippet(subject, snippet)
		if ok {
			if fullRide, err := p.Parse(subject, body); err == nil {
				if ride.PickupAddress == "" {
					ride.PickupAddress = fullRide.PickupAddress
				}
				if ride.DropAddress == "" {
					ride.DropAddress = fullRide.DropAddress
				}
				if ride.PickupLat == nil {
					ride.PickupLat, ride.PickupLng = fullRide.PickupLat, fullRide.PickupLng
				}
				if ride.DropLat == nil {
					ride.DropLat, ride.DropLng = fullRide.DropLat, fullRide.DropLng
				}
			}
			// Merge HTML coords as final fallback.
			if ride.PickupLat == nil {
				ride.PickupLat, ride.PickupLng = htmlPickupLat, htmlPickupLng
			}
			if ride.DropLat == nil {
				ride.DropLat, ride.DropLng = htmlDropLat, htmlDropLng
			}
			resolved := resolveCode(candidates, ride.ProviderEmailTypeCode, candidate.Code)
			return s.ingestRide(ctx, userID, messageID, resolved, candidate, dateHeader, subject, snippet, ride)
		}

		// ── Full-body parse ───────────────────────────────────────────────────
		fullRide, err := p.Parse(subject, body)
		if errors.Is(err, parser.ErrCancellation) {
			return s.skipOutcome(userID, messageID, &candidate.Code, subject, snippet)
		}
		if errors.Is(err, parser.ErrUnrecognisedFormat) {
			continue // try next candidate
		}
		if err != nil {
			return s.failOutcome(userID, messageID, &candidate.Code, subject, snippet, err)
		}

		// Merge HTML-extracted coords into the ride if the parser didn't find any.
		if fullRide.PickupLat == nil {
			fullRide.PickupLat, fullRide.PickupLng = htmlPickupLat, htmlPickupLng
		}
		if fullRide.DropLat == nil {
			fullRide.DropLat, fullRide.DropLng = htmlDropLat, htmlDropLng
		}

		resolved := resolveCode(candidates, fullRide.ProviderEmailTypeCode, candidate.Code)
		return s.ingestRide(ctx, userID, messageID, resolved, candidate, dateHeader, subject, snippet, fullRide)
	}

	// All candidates tried and none succeeded. LLM already ran (or is disabled).
	return s.failOutcome(userID, messageID, &candidates[0].Code, subject, snippet,
		fmt.Errorf("%w: no parser matched body", parser.ErrUnrecognisedFormat))
}

// ingestRide creates an activity from the parsed ride and returns a log outcome.
func (s *GmailService) ingestRide(
	ctx context.Context,
	userID uuid.UUID,
	messageID string,
	resolvedCode string,
	candidate domain.ProviderEmailType,
	dateHeader, subject, snippet string,
	ride *parser.ParsedRide,
) msgOutcome {
	activity, err := s.buildAndIngest(ctx, userID, messageID, candidate.ProviderCode, dateHeader, ride)
	if errors.Is(err, actservice.ErrDuplicate) {
		return s.skipOutcome(userID, messageID, &resolvedCode, subject, snippet)
	}
	if err != nil {
		return s.failOutcome(userID, messageID, &resolvedCode, subject, snippet,
			fmt.Errorf("ingest activity: %w", err))
	}
	logEntry := s.newLog(userID, messageID, &resolvedCode, subject, snippet, domain.StatusParsed)
	logEntry.ActivityID = &activity.ID
	return msgOutcome{messageID: messageID, log: logEntry, counted: true}
}

// buildAndIngest geocodes addresses and calls IngestWithDedup.
// Extracted so that both the sync path (ingestRide) and the async LLM enrichment
// path (EnrichUnrecognised) can share the same ingestion logic without coupling
// them to the log-outcome pattern.
func (s *GmailService) buildAndIngest(
	ctx context.Context,
	userID uuid.UUID,
	messageID string,
	providerCode string,
	dateHeader string,
	ride *parser.ParsedRide,
) (*actdomain.Activity, error) {
	log := logger.L()

	startedAt := ride.StartedAt
	if startedAt.IsZero() {
		startedAt = parseEmailDate(dateHeader)
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	mode := actdomain.TransportMode(ride.TransportMode)
	actType := actdomain.ActivityTransport
	if ride.TransportMode == "flight" {
		actType = actdomain.ActivityFlight
	}

	// Geocode pickup and drop concurrently — two independent HTTP calls.
	type geoResult struct{ lat, lng *float64 }
	pickupCh := make(chan geoResult, 1)
	dropCh := make(chan geoResult, 1)

	go func() {
		res := geoResult{ride.PickupLat, ride.PickupLng}
		if res.lat == nil && ride.PickupAddress != "" {
			if lat, lng, err := s.geocoder.Geocode(ctx, ride.PickupAddress); err == nil {
				res.lat, res.lng = &lat, &lng
			} else {
				log.Debug("gmail: geocode pickup failed", zap.String("address", ride.PickupAddress), zap.Error(err))
			}
		}
		pickupCh <- res
	}()
	go func() {
		res := geoResult{ride.DropLat, ride.DropLng}
		if res.lat == nil && ride.DropAddress != "" {
			if lat, lng, err := s.geocoder.Geocode(ctx, ride.DropAddress); err == nil {
				res.lat, res.lng = &lat, &lng
			} else {
				log.Debug("gmail: geocode drop failed", zap.String("address", ride.DropAddress), zap.Error(err))
			}
		}
		dropCh <- res
	}()
	pickupRes := <-pickupCh
	dropRes := <-dropCh

	var endedAt *time.Time
	if ride.DurationMinutes != nil {
		end := startedAt.Add(time.Duration(*ride.DurationMinutes) * time.Minute)
		endedAt = &end
	}

	fareCurrency := ride.Currency
	if fareCurrency == "" && ride.FareAmount != nil {
		fareCurrency = "INR"
	}
	var fareCurrencyPtr *string
	if fareCurrency != "" {
		fareCurrencyPtr = &fareCurrency
	}

	receiptID := messageID
	var originPtr, destinationPtr *string
	if ride.PickupAddress != "" {
		originPtr = &ride.PickupAddress
	}
	if ride.DropAddress != "" {
		destinationPtr = &ride.DropAddress
	}

	activity, _, err := s.activitySvc.IngestWithDedup(ctx, actservice.IngestInput{
		UserID:          userID,
		ActivityType:    actType,
		TransportMode:   &mode,
		DistanceKM:      &ride.DistanceKM,
		DurationMinutes: ride.DurationMinutes,
		Source:          actdomain.SourceGmail,
		Provider:        &providerCode,
		RawMetadata:     actdomain.RawMetadata(ride.Metadata),
		StartedAt:       startedAt,
		EndedAt:         endedAt,
		UserTimezone:    "UTC",
		IdempotencyKey:  "gmail:" + messageID,
		Origin:          originPtr,
		Destination:     destinationPtr,
		OriginLat:       pickupRes.lat,
		OriginLng:       pickupRes.lng,
		DestLat:         dropRes.lat,
		DestLng:         dropRes.lng,
		ReceiptID:       &receiptID,
		FareAmount:      ride.FareAmount,
		FareCurrency:    fareCurrencyPtr,
	})
	return activity, err
}

// ── LLM enrichment ────────────────────────────────────────────────────────────

// EnrichResult summarises one LLM enrichment run for a single user.
type EnrichResult struct {
	Total    int `json:"total"`
	Enriched int `json:"enriched"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
}

// AllEnrichResult summarises an EnrichUnrecognisedAll run.
type AllEnrichResult struct {
	TotalUsers  int    `json:"total_users"`
	Enriched    int    `json:"enriched"`
	Failed      int    `json:"failed"`       // email-level parse/ingest failures
	UsersFailed int    `json:"users_failed"` // users whose auth or DB setup failed
	Error       string `json:"error,omitempty"`
}

// EnrichUnrecognised re-processes emails that previously failed regex parsing
// via the Groq API. Each successfully parsed email creates an activity and
// updates the existing log entry from "unrecognised" to "parsed".
func (s *GmailService) EnrichUnrecognised(ctx context.Context, userID uuid.UUID) (*EnrichResult, error) {
	conn, err := s.connRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, ErrNotConnected
	}

	gmailSvc, err := s.gmailClient(ctx, conn)
	if err != nil {
		return nil, err // gmailClient wraps revoked-token errors as ErrNotConnected
	}

	routingMap, err := s.provRepo.BuildRoutingMap(ctx)
	if err != nil {
		return nil, fmt.Errorf("build routing map: %w", err)
	}

	return s.enrichUser(ctx, userID, gmailSvc, routingMap)
}

// enrichUser processes the unrecognised queue for one user given an already-authenticated
// Gmail client and a pre-built routing map. Splitting this from EnrichUnrecognised lets
// EnrichUnrecognisedAll build the routing map once for all users.
func (s *GmailService) enrichUser(
	ctx context.Context,
	userID uuid.UUID,
	gmailSvc *googleapi.Service,
	routingMap repository.RoutingMap,
) (*EnrichResult, error) {
	log := logger.L()

	unrecognised, err := s.logRepo.ListUnrecognised(ctx, userID, 50)
	if err != nil {
		return nil, fmt.Errorf("list unrecognised: %w", err)
	}

	result := &EnrichResult{Total: len(unrecognised)}
	if len(unrecognised) == 0 {
		return result, nil
	}

	// updateLog writes a status update using a context that is not bound to the
	// HTTP request lifecycle. This prevents a client disconnect from leaving a
	// log entry in a stale state (e.g. permanently StatusFailed) when the DB
	// write itself would have succeeded.
	updateLog := func(id uuid.UUID, status string, activityID *uuid.UUID) {
		writeCtx := context.WithoutCancel(ctx)
		if err := s.logRepo.UpdateStatus(writeCtx, id, status, activityID); err != nil {
			log.Warn("gmail enrich: update log status failed",
				zap.Stringer("log_id", id), zap.String("status", status), zap.Error(err))
		}
	}

	for _, entry := range unrecognised {
		msg, err := gmailSvc.Users.Messages.Get("me", entry.MessageID).
			Format("full").Context(ctx).Do()
		if err != nil {
			// Don't mark StatusFailed on a fetch error — a token problem would
			// poison every entry in the batch. Leave as StatusUnrecognised so
			// the next cron run can retry once auth is restored.
			log.Warn("gmail enrich: fetch message failed",
				zap.String("msg_id", entry.MessageID), zap.Error(err))
			result.Failed++
			continue
		}

		fromRaw, subject, dateHeader := extractHeaders(msg)
		from := extractEmailAddress(fromRaw)

		// Build the content to send to the LLM. When the email has a plain-text
		// part, append the raw HTML so the model can also see map URLs and
		// structured addresses. When there is no plain-text part, send the raw
		// HTML directly — extractBody would return stripHTML(rawHTML), which
		// would just double the payload size with redundant content.
		//
		// Truncate plainText to 5000 bytes before concatenating so the
		// rawHTML (which carries map coordinate URLs) always fits within
		// the 8000-byte limit applied by llmTruncate inside invoke().
		rawHTML := extractPart(msg.Payload, "text/html")
		plainText := extractPart(msg.Payload, "text/plain")
		// Strip style/script blocks from the HTML before appending — HTML-only
		// emails (e.g. Uber) embed ~3000 chars of CSS that would consume the
		// entire token budget before any visible content is reached.
		strippedHTML := stripHTML(rawHTML)
		var fullContent string
		if plainText != "" {
			// Cap plain-text at 5000 bytes so strippedHTML always fits
			// within the 8000-byte limit applied by llmTruncate inside invoke().
			if len(plainText) > 5000 {
				plainText = plainText[:5000]
			}
			fullContent = plainText + "\n\n" + strippedHTML
		} else {
			fullContent = strippedHTML
		}

		ride, err := s.llmParser.ParseWithContext(ctx, subject, fullContent)
		if errors.Is(err, parser.ErrUnrecognisedFormat) {
			// LLM confirmed this is not a ride receipt — skip it, don't fail.
			updateLog(entry.ID, domain.StatusSkipped, nil)
			result.Skipped++
			continue
		}
		if err != nil {
			// Transient error (network, rate-limit, etc.) — leave as StatusUnrecognised
			// so the next cron run can retry, matching how Gmail API fetch errors are handled.
			log.Warn("gmail enrich: llm parse failed",
				zap.String("msg_id", entry.MessageID), zap.Error(err))
			result.Failed++
			continue
		}

		// Merge HTML-extracted map coords as fallback if LLM didn't return any.
		htmlPickupLat, htmlPickupLng, htmlDropLat, htmlDropLng := parser.ExtractMapCoords(rawHTML)
		if ride.PickupLat == nil {
			ride.PickupLat, ride.PickupLng = htmlPickupLat, htmlPickupLng
		}
		if ride.DropLat == nil {
			ride.DropLat, ride.DropLng = htmlDropLat, htmlDropLng
		}

		// Determine provider code: prefer routing map over LLM metadata guess.
		providerCode := "unknown"
		candidates := filterBySubject(routingMap[strings.ToLower(from)], subject)
		if len(candidates) > 0 {
			providerCode = candidates[0].ProviderCode
		} else if pc, ok := ride.Metadata["provider"].(string); ok && pc != "" {
			providerCode = pc
		}

		activity, err := s.buildAndIngest(ctx, userID, entry.MessageID, providerCode, dateHeader, ride)
		if errors.Is(err, actservice.ErrDuplicate) {
			// Activity already exists (e.g. cron ran twice before log update).
			updateLog(entry.ID, domain.StatusSkipped, nil)
			result.Skipped++
			continue
		}
		if err != nil {
			log.Warn("gmail enrich: ingest failed",
				zap.String("msg_id", entry.MessageID), zap.Error(err))
			updateLog(entry.ID, domain.StatusFailed, nil)
			result.Failed++
			continue
		}

		updateLog(entry.ID, domain.StatusParsed, &activity.ID)
		result.Enriched++
	}

	log.Info("gmail enrich: user complete",
		zap.Stringer("user_id", userID),
		zap.Int("total", result.Total),
		zap.Int("enriched", result.Enriched),
		zap.Int("failed", result.Failed),
	)
	return result, nil
}

// EnrichUnrecognisedAll runs enrichUser for every connected user.
// Called by the worker cron and POST /internal/gmail/enrich-all.
// The routing map is built once and shared across all users to avoid N redundant queries.
func (s *GmailService) EnrichUnrecognisedAll(ctx context.Context) *AllEnrichResult {
	log := logger.L()

	conns, err := s.connRepo.FindAllConnected(ctx)
	if err != nil {
		log.Error("gmail enrich all: list connections", zap.Error(err))
		return &AllEnrichResult{Error: err.Error()}
	}

	routingMap, err := s.provRepo.BuildRoutingMap(ctx)
	if err != nil {
		log.Error("gmail enrich all: build routing map", zap.Error(err))
		return &AllEnrichResult{TotalUsers: len(conns), Error: err.Error()}
	}

	res := &AllEnrichResult{TotalUsers: len(conns)}
	for i := range conns {
		gmailSvc, err := s.gmailClient(ctx, &conns[i])
		if err != nil {
			log.Warn("gmail enrich all: auth failed",
				zap.Stringer("user_id", conns[i].UserID), zap.Error(err))
			res.UsersFailed++
			continue
		}
		result, err := s.enrichUser(ctx, conns[i].UserID, gmailSvc, routingMap)
		if err != nil {
			log.Warn("gmail enrich all: user failed",
				zap.Stringer("user_id", conns[i].UserID), zap.Error(err))
			res.UsersFailed++
			continue
		}
		res.Enriched += result.Enriched
		res.Failed += result.Failed
	}

	log.Info("gmail enrich all: complete",
		zap.Int("total_users", res.TotalUsers),
		zap.Int("enriched", res.Enriched),
		zap.Int("emails_failed", res.Failed),
		zap.Int("users_failed", res.UsersFailed),
	)
	return res
}

// tryBackfillAddresses fetches and re-parses a previously ingested email to
// populate origin/destination on its activity when those fields are NULL.
// Errors are silently logged — this is best-effort enrichment only.
func (s *GmailService) tryBackfillAddresses(
	ctx context.Context,
	gmailSvc *googleapi.Service,
	userID uuid.UUID,
	messageID string,
	activityID uuid.UUID,
	routingMap repository.RoutingMap,
) {
	msg, err := gmailSvc.Users.Messages.Get("me", messageID).Format("full").Context(ctx).Do()
	if err != nil {
		logger.L().Warn("gmail: backfill fetch failed", zap.String("msg_id", messageID), zap.Error(err))
		return
	}

	fromRaw, subject, _ := extractHeaders(msg)
	from := extractEmailAddress(fromRaw)
	candidates := filterBySubject(routingMap[strings.ToLower(from)], subject)
	if len(candidates) == 0 {
		return
	}

	body := extractBody(msg)
	for _, candidate := range candidates {
		p, ok := s.registry.Get(candidate.Code)
		if !ok {
			continue
		}
		ride, err := p.Parse(subject, body)
		if err != nil {
			continue
		}
		if ride.PickupAddress == "" && ride.DropAddress == "" {
			return
		}
		if err := s.activitySvc.BackfillRouteLabels(ctx, activityID, ride.PickupAddress, ride.DropAddress); err != nil {
			logger.L().Warn("gmail: backfill route labels failed",
				zap.String("activity_id", activityID.String()), zap.Error(err))
		}
		return
	}
}

// ── Outcome helpers ───────────────────────────────────────────────────────────

func (s *GmailService) skipOutcome(userID uuid.UUID, msgID string, code *string, subject, snippet string) msgOutcome {
	return msgOutcome{
		messageID: msgID,
		log:       s.newLog(userID, msgID, code, subject, snippet, domain.StatusSkipped),
		counted:   true,
	}
}

// unrecognisedOutcome records the email as pending LLM enrichment.
// The worker cron picks these up via EnrichUnrecognisedAll.
func (s *GmailService) unrecognisedOutcome(userID uuid.UUID, msgID string, code *string, subject, snippet string) msgOutcome {
	return msgOutcome{
		messageID: msgID,
		log:       s.newLog(userID, msgID, code, subject, snippet, domain.StatusUnrecognised),
		counted:   true,
	}
}

func (s *GmailService) failOutcome(userID uuid.UUID, msgID string, code *string, subject, snippet string, err error) msgOutcome {
	l := s.newLog(userID, msgID, code, subject, snippet, domain.StatusFailed)
	errStr := err.Error()
	l.ErrorReason = &errStr
	return msgOutcome{messageID: msgID, log: l, counted: true}
}

func (s *GmailService) newLog(userID uuid.UUID, msgID string, code *string, subject, snippet, status string) *domain.EmailIngestionLog {
	return &domain.EmailIngestionLog{
		ID:         uuidpkg.New(),
		UserID:     userID,
		MessageID:  msgID,
		SenderCode: code,
		Subject:    subject,
		Snippet:    snippet,
		Status:     status,
		ParsedAt:   time.Now().UTC(),
	}
}

// ── Routing helpers ───────────────────────────────────────────────────────────

// filterBySubject removes candidates whose subject_pattern does not match.
// Candidates with no pattern (NULL) always pass.
func filterBySubject(candidates []domain.ProviderEmailType, subject string) []domain.ProviderEmailType {
	var out []domain.ProviderEmailType
	for _, c := range candidates {
		if c.SubjectPattern == nil || *c.SubjectPattern == "" {
			out = append(out, c)
			continue
		}
		matched, _ := regexp.MatchString(*c.SubjectPattern, subject)
		if matched {
			out = append(out, c)
		}
	}
	return out
}

// resolveCode returns the parser-returned code when valid, otherwise falls
// back to the candidate code. This handles cases where one parser covers
// multiple codes (e.g. RapidoParser returns "rapido_bike" or "rapido_auto").
func resolveCode(candidates []domain.ProviderEmailType, parserCode, fallback string) string {
	if parserCode == "" {
		return fallback
	}
	for _, c := range candidates {
		if c.Code == parserCode {
			return parserCode
		}
	}
	return fallback
}

// ── Gmail API client ──────────────────────────────────────────────────────────

// gmailClient builds an authenticated Gmail API client.
// If the stored access token has expired, the oauth2 library refreshes it
// transparently; we then persist the new token back to DB.
func (s *GmailService) gmailClient(ctx context.Context, conn *domain.GmailConnection) (*googleapi.Service, error) {
	stored := &oauth2.Token{
		AccessToken:  conn.AccessToken,
		RefreshToken: conn.RefreshToken,
		Expiry:       conn.TokenExpiry,
	}
	ts := s.oauthCfg.TokenSource(ctx, stored)

	// Token() triggers a refresh when the access token is expired.
	// A refresh failure means the connection is broken (revoked grant), so we
	// wrap as ErrNotConnected so all callers can surface a meaningful error.
	fresh, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: token refresh: %v", ErrNotConnected, err)
	}
	if fresh.AccessToken != conn.AccessToken {
		conn.AccessToken = fresh.AccessToken
		conn.TokenExpiry = fresh.Expiry.UTC()
		conn.UpdatedAt = time.Now().UTC()
		_ = s.connRepo.Save(ctx, conn)
	}

	return googleapi.NewService(ctx, option.WithTokenSource(ts))
}

func (s *GmailService) fetchGmailEmail(ctx context.Context, token *oauth2.Token) (string, error) {
	ts := s.oauthCfg.TokenSource(ctx, token)
	svc, err := googleapi.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", err
	}
	profile, err := svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return profile.EmailAddress, nil
}

// ── OAuth state signing ───────────────────────────────────────────────────────

type statePayload struct {
	UserID   string `json:"u"`
	IssuedAt int64  `json:"t"`
	// Platform is "mobile" when the flow originates from the mobile app.
	// Empty / absent means web.
	Platform string `json:"p,omitempty"`
}

func (s *GmailService) signState(userID, platform string) string {
	p := statePayload{UserID: userID, IssuedAt: time.Now().Unix(), Platform: platform}
	b, _ := json.Marshal(p)
	enc := base64.RawURLEncoding.EncodeToString(b)
	return enc + "." + s.hmacSign(enc)
}

// verifyState returns (userID, platform, error).
func (s *GmailService) verifyState(state string) (string, string, error) {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return "", "", errors.New("malformed state")
	}
	if !hmac.Equal([]byte(s.hmacSign(parts[0])), []byte(parts[1])) {
		return "", "", errors.New("state signature invalid")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", err
	}
	var p statePayload
	if err := json.Unmarshal(b, &p); err != nil {
		return "", "", err
	}
	if time.Since(time.Unix(p.IssuedAt, 0)) > 10*time.Minute {
		return "", "", errors.New("state expired")
	}
	return p.UserID, p.Platform, nil
}

func (s *GmailService) hmacSign(data string) string {
	h := hmac.New(sha256.New, s.hmacSecret)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// ── Email body / header extraction ───────────────────────────────────────────

// extractEmailAddress strips the display-name portion from a MIME From header.
// "Uber Receipts India <noreply@uber.com>" → "noreply@uber.com"
// "noreply@uber.com"                       → "noreply@uber.com"
// Returns the original string if parsing fails (best-effort).
func extractEmailAddress(raw string) string {
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		// Fall back to the raw value; ToLower + routing map lookup will miss
		// anything with a display name, but at least we don't lose bare addresses.
		return strings.TrimSpace(raw)
	}
	return addr.Address
}

func extractHeaders(msg *googleapi.Message) (from, subject, date string) {
	if msg.Payload == nil {
		return
	}
	for _, h := range msg.Payload.Headers {
		switch strings.ToLower(h.Name) {
		case "from":
			from = h.Value
		case "subject":
			subject = h.Value
		case "date":
			date = h.Value
		}
	}
	return
}

// extractBody returns plain text, falling back to HTML→text stripping.
func extractBody(msg *googleapi.Message) string {
	if t := extractPart(msg.Payload, "text/plain"); t != "" {
		return t
	}
	return stripHTML(extractPart(msg.Payload, "text/html"))
}

func extractPart(part *googleapi.MessagePart, mime string) string {
	if part == nil {
		return ""
	}
	if strings.HasPrefix(part.MimeType, mime) && part.Body != nil && part.Body.Data != "" {
		if b, err := base64.URLEncoding.DecodeString(part.Body.Data); err == nil {
			return string(b)
		}
	}
	for _, sub := range part.Parts {
		if t := extractPart(sub, mime); t != "" {
			return t
		}
	}
	return ""
}

var (
	reHTMLTag     = regexp.MustCompile(`<[^>]+>`)
	reHTMLNbsp    = regexp.MustCompile(`&nbsp;`)
	reHTMLAmp     = regexp.MustCompile(`&amp;`)
	reBlankLines  = regexp.MustCompile(`\n{3,}`)
	reStyleBlock  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reScriptBlock = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
)

func stripHTML(html string) string {
	// Remove style/script blocks first — their content is not visible text and
	// would otherwise consume the entire token budget (Uber emails have ~3000
	// chars of CSS that precedes any actual email content).
	s := reStyleBlock.ReplaceAllString(html, "")
	s = reScriptBlock.ReplaceAllString(s, "")
	s = reHTMLNbsp.ReplaceAllString(s, " ")
	s = reHTMLAmp.ReplaceAllString(s, "&")
	s = reHTMLTag.ReplaceAllString(s, " ")
	s = reBlankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

var emailDateLayouts = []string{
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	time.RFC1123Z,
	time.RFC1123,
}

func parseEmailDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range emailDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
