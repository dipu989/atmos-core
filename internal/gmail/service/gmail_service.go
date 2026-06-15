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
		hmacSecret:  []byte(cfg.HMACSecret),
		batchSize:   batchSize,
	}
}

// ── OAuth connect / disconnect ────────────────────────────────────────────────

// AuthURL generates the Google consent-page URL.
// The userID is embedded in a signed state parameter — no session cookie needed.
func (s *GmailService) AuthURL(userID uuid.UUID) string {
	return s.oauthCfg.AuthCodeURL(
		s.signState(userID.String()),
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)
}

// HandleCallback exchanges the OAuth code for tokens and persists the connection.
func (s *GmailService) HandleCallback(ctx context.Context, state, code string) (*domain.GmailConnection, error) {
	userIDStr, err := s.verifyState(state)
	if err != nil {
		return nil, fmt.Errorf("invalid oauth state: %w", err)
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid user id in state: %w", err)
	}
	token, err := s.oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	email, err := s.fetchGmailEmail(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("fetch gmail profile: %w", err)
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
		return nil, fmt.Errorf("save gmail connection: %w", err)
	}
	return conn, nil
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
	processed, err := s.logRepo.IsProcessed(ctx, userID, messageID)
	if err != nil {
		logger.L().Warn("gmail: idempotency check failed", zap.Error(err))
	}
	if processed {
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

	// ── Snippet-first parse ───────────────────────────────────────────────────
	// Try the snippet before decoding the full body — saves CPU for Uber emails
	// whose snippets already contain vehicle type + distance + duration.
	body := "" // lazy — only built if snippet parse fails
	for _, candidate := range candidates {
		p, ok := s.registry.Get(candidate.Code)
		if !ok {
			continue
		}

		if ride, ok := p.TrySnippet(subject, snippet); ok {
			// Snippet was enough — no body decoding needed.
			resolved := resolveCode(candidates, ride.ProviderEmailTypeCode, candidate.Code)
			return s.ingestRide(ctx, userID, messageID, resolved, candidate, dateHeader, subject, snippet, ride)
		}
	}

	// ── Full-body parse ───────────────────────────────────────────────────────
	// Extract raw HTML first so we can scan it for embedded map coordinates
	// (they exist in <a href> attributes and are lost once HTML is stripped).
	rawHTML := extractPart(msg.Payload, "text/html")
	htmlPickupLat, htmlPickupLng, htmlDropLat, htmlDropLng := parser.ExtractMapCoords(rawHTML)

	body = extractBody(msg)
	for _, candidate := range candidates {
		p, ok := s.registry.Get(candidate.Code)
		if !ok {
			continue
		}

		ride, err := p.Parse(subject, body)
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
		if ride.PickupLat == nil {
			ride.PickupLat, ride.PickupLng = htmlPickupLat, htmlPickupLng
		}
		if ride.DropLat == nil {
			ride.DropLat, ride.DropLng = htmlDropLat, htmlDropLng
		}

		resolved := resolveCode(candidates, ride.ProviderEmailTypeCode, candidate.Code)
		return s.ingestRide(ctx, userID, messageID, resolved, candidate, dateHeader, subject, snippet, ride)
	}

	// All candidates tried and none succeeded.
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
	log := logger.L()

	// Fall back to email Date: header when parser couldn't extract a date.
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
	pickupLat, pickupLng := pickupRes.lat, pickupRes.lng
	dropLat, dropLng := dropRes.lat, dropRes.lng

	// Compute a derived EndedAt from DurationMinutes when we have a start time.
	var endedAt *time.Time
	if ride.DurationMinutes != nil {
		end := startedAt.Add(time.Duration(*ride.DurationMinutes) * time.Minute)
		endedAt = &end
	}

	// Fare currency defaults to INR (all current providers are India-based).
	fareCurrency := ride.Currency
	if fareCurrency == "" && ride.FareAmount != nil {
		fareCurrency = "INR"
	}
	var fareCurrencyPtr *string
	if fareCurrency != "" {
		fareCurrencyPtr = &fareCurrency
	}

	// IngestWithDedup runs the TripMatcher before creating a row:
	// if a GPS activity from the same trip already exists it is enriched in place;
	// otherwise a new receipt-sourced activity is created.
	// Layer 2 idempotency: "gmail:<messageId>" ensures replay safety.
	receiptID := messageID
	activity, _, err := s.activitySvc.IngestWithDedup(ctx, actservice.IngestInput{
		UserID:          userID,
		ActivityType:    actType,
		TransportMode:   &mode,
		DistanceKM:      &ride.DistanceKM,
		DurationMinutes: ride.DurationMinutes,
		Source:          actdomain.SourceGmail,
		Provider:        &candidate.ProviderCode, // "uber", "rapido"
		RawMetadata:     actdomain.RawMetadata(ride.Metadata),
		StartedAt:       startedAt,
		EndedAt:         endedAt,
		UserTimezone:    "UTC",
		IdempotencyKey:  "gmail:" + messageID,
		OriginLat:       pickupLat,
		OriginLng:       pickupLng,
		DestLat:         dropLat,
		DestLng:         dropLng,
		ReceiptID:       &receiptID,
		FareAmount:      ride.FareAmount,
		FareCurrency:    fareCurrencyPtr,
	})
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

// ── Outcome helpers ───────────────────────────────────────────────────────────

func (s *GmailService) skipOutcome(userID uuid.UUID, msgID string, code *string, subject, snippet string) msgOutcome {
	return msgOutcome{
		messageID: msgID,
		log:       s.newLog(userID, msgID, code, subject, snippet, domain.StatusSkipped),
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
	fresh, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("token refresh: %w", err)
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
}

func (s *GmailService) signState(userID string) string {
	p := statePayload{UserID: userID, IssuedAt: time.Now().Unix()}
	b, _ := json.Marshal(p)
	enc := base64.RawURLEncoding.EncodeToString(b)
	return enc + "." + s.hmacSign(enc)
}

func (s *GmailService) verifyState(state string) (string, error) {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return "", errors.New("malformed state")
	}
	if !hmac.Equal([]byte(s.hmacSign(parts[0])), []byte(parts[1])) {
		return "", errors.New("state signature invalid")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	var p statePayload
	if err := json.Unmarshal(b, &p); err != nil {
		return "", err
	}
	if time.Since(time.Unix(p.IssuedAt, 0)) > 10*time.Minute {
		return "", errors.New("state expired")
	}
	return p.UserID, nil
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
	reHTMLTag    = regexp.MustCompile(`<[^>]+>`)
	reHTMLNbsp   = regexp.MustCompile(`&nbsp;`)
	reHTMLAmp    = regexp.MustCompile(`&amp;`)
	reBlankLines = regexp.MustCompile(`\n{3,}`)
)

func stripHTML(html string) string {
	s := reHTMLNbsp.ReplaceAllString(html, " ")
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
