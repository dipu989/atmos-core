package handler

import (
	"errors"

	"github.com/dipu/atmos-core/internal/gmail/dto"
	"github.com/dipu/atmos-core/internal/gmail/service"
	"github.com/dipu/atmos-core/platform/middleware"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
)

// GmailHandler exposes Gmail connect / sync endpoints.
type GmailHandler struct {
	svc *service.GmailService
}

func NewGmailHandler(svc *service.GmailService) *GmailHandler {
	return &GmailHandler{svc: svc}
}

// Connect godoc
// @Summary     Start Gmail OAuth flow
// @Description Redirects the authenticated user to Google's consent page.
//
//	After granting permission, Google redirects back to /gmail/callback.
//
// @Tags        gmail
// @Security    BearerAuth
// @Success     302
// @Failure     401 {object} map[string]interface{}
// @Router      /gmail/connect [get]
func (h *GmailHandler) Connect(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	authURL := h.svc.AuthURL(userID)
	return c.Redirect(authURL, fiber.StatusFound)
}

// AuthURL godoc
// @Summary     Get Gmail OAuth URL
// @Description Returns the Google consent-page URL as JSON so mobile clients
//
//	can open it in the system browser without a server-side redirect.
//	Pass ?platform=mobile to embed a mobile platform hint in the signed
//	state; the callback will then redirect to atmos://gmail/connected
//	so the app can re-enter the foreground with the connection confirmed.
//
// @Tags        gmail
// @Produce     json
// @Security    BearerAuth
// @Param       platform query string false "Target platform: mobile (deep-link callback) or omit for web"
// @Success     200 {object} map[string]interface{}
// @Failure     401 {object} map[string]interface{}
// @Router      /gmail/auth-url [get]
func (h *GmailHandler) AuthURL(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	platform := c.Query("platform") // "mobile" or ""
	// Only "mobile" is a valid non-empty platform value; reject anything else so a
	// misconfigured client doesn't silently receive a web redirect after OAuth.
	if platform != "" && platform != "mobile" {
		return response.BadRequest(c, `invalid platform: use "mobile" or omit`)
	}
	url := h.svc.AuthURLForPlatform(userID, platform)
	if url == "" {
		return response.InternalError(c, "gmail OAuth not configured")
	}
	return response.OK(c, fiber.Map{"url": url})
}

// Callback godoc
// @Summary     Gmail OAuth callback
// @Description Exchanges the auth code for tokens and stores them.
//
//	Google redirects here after the user grants permission.
//	On success, redirects to atmos://gmail/connected for mobile flows,
//	or to the frontend's settings page for web flows.
//	On error, redirects to atmos://gmail/error for mobile flows so the
//	app can return to the foreground and surface the failure gracefully.
//
// @Tags        gmail
// @Param       state query string true "OAuth state"
// @Param       code  query string true "OAuth code"
// @Success     302
// @Failure     400 {object} map[string]interface{}
// @Router      /gmail/callback [get]
func (h *GmailHandler) Callback(c *fiber.Ctx) error {
	state := c.Query("state")
	code := c.Query("code")
	if state == "" || code == "" {
		return response.BadRequest(c, "missing state or code")
	}

	_, platform, err := h.svc.HandleCallback(c.Context(), state, code)
	if err != nil {
		if platform == "mobile" {
			// Redirect the mobile app back to the foreground with an error signal so
			// the user is not left stranded on a JSON error page in the browser.
			return c.Redirect("atmos://gmail/error", fiber.StatusFound)
		}
		return response.BadRequest(c, "gmail connect failed: "+err.Error())
	}

	if platform == "mobile" {
		// Deep-link the mobile app back to the foreground with success signal.
		return c.Redirect("atmos://gmail/connected", fiber.StatusFound)
	}
	return c.Redirect("/", fiber.StatusFound)
}

// Status godoc
// @Summary     Gmail connection status
// @Description Returns whether the user has connected their Gmail account.
// @Tags        gmail
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} dto.ConnectionStatus
// @Failure     401 {object} map[string]interface{}
// @Router      /gmail/status [get]
func (h *GmailHandler) Status(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	conn, err := h.svc.Status(c.Context(), userID)
	if err != nil {
		return response.InternalError(c, "could not fetch gmail status")
	}
	return response.OK(c, dto.ConnectionStatusFromDomain(conn))
}

// Disconnect godoc
// @Summary     Disconnect Gmail
// @Description Removes the stored Gmail OAuth tokens for the authenticated user.
// @Tags        gmail
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} map[string]interface{}
// @Failure     401 {object} map[string]interface{}
// @Router      /gmail/disconnect [delete]
func (h *GmailHandler) Disconnect(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	if err := h.svc.Disconnect(c.Context(), userID); err != nil {
		return response.InternalError(c, "could not disconnect gmail")
	}
	return response.OK(c, fiber.Map{"message": "gmail disconnected"})
}

// Sync godoc
// @Summary     Trigger Gmail email sync
// @Description Fetches recent ride-receipt emails, parses them, and ingests activities.
//
//	Safe to call multiple times — already-processed emails are skipped.
//
// @Tags        gmail
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} dto.SyncResponse
// @Failure     401 {object} map[string]interface{}
// @Failure     422 {object} map[string]interface{}
// @Router      /gmail/sync [post]
func (h *GmailHandler) Sync(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	result, err := h.svc.Sync(c.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrNotConnected) {
			return response.BadRequest(c, "gmail not connected — call /gmail/connect first")
		}
		return response.InternalError(c, "sync failed: "+err.Error())
	}

	return response.OK(c, dto.SyncResponse{
		MessagesChecked: result.MessagesChecked,
		Parsed:          result.Parsed,
		Skipped:         result.Skipped,
		Failed:          result.Failed,
		Unrecognised:    result.Unrecognised,
		Message:         "sync complete",
	})
}

// ResetSync godoc
// @Summary     Reset Gmail sync history
// @Description Clears the stored Gmail historyId so the next sync re-scans all
//
//	emails from the initial lookback window (90 days). Use this after a
//	parser fix to backfill fields (e.g. origin/destination) that were
//	missing on earlier syncs.
//
// @Tags        gmail
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} map[string]interface{}
// @Failure     401 {object} map[string]interface{}
// @Failure     422 {object} map[string]interface{}
// @Router      /gmail/reset-sync [post]
func (h *GmailHandler) ResetSync(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	if err := h.svc.ResetSync(c.Context(), userID); err != nil {
		if errors.Is(err, service.ErrNotConnected) {
			return response.BadRequest(c, "gmail not connected")
		}
		return response.InternalError(c, "could not reset sync history")
	}
	return response.OK(c, fiber.Map{"message": "sync history reset — next sync will re-scan all emails"})
}

// SyncAll godoc
// @Summary     Trigger Gmail sync for all users
// @Description Internal endpoint called by the Linux cron job to sync Gmail for every
//
//	connected user. Protected by X-Internal-Key header. Applies a 23-hour
//	cooldown per user so re-running the cron is always safe.
//
// @Tags        internal
// @Produce     json
// @Param       X-Internal-Key header string true "Internal shared secret"
// @Success     200 {object} map[string]interface{}
// @Failure     401 {object} map[string]interface{}
// @Router      /internal/gmail/sync-all [post]
func (h *GmailHandler) SyncAll(c *fiber.Ctx) error {
	result := h.svc.SyncAll(c.Context())
	return response.OK(c, result)
}

// EnrichUnrecognised godoc
// @Summary     Re-parse unrecognised emails via LLM
// @Description Fetches emails that failed regex parsing (status=unrecognised) and
//
//	re-processes them using the Groq API. Requires GROQ_API_KEY to be set.
//
// @Tags        gmail
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} map[string]interface{}
// @Failure     401 {object} map[string]interface{}
// @Failure     422 {object} map[string]interface{}
// @Router      /gmail/enrich-unrecognised [post]
func (h *GmailHandler) EnrichUnrecognised(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	result, err := h.svc.EnrichUnrecognised(c.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrNotConnected) {
			return response.BadRequest(c, "gmail not connected — call /gmail/connect first")
		}
		return response.InternalError(c, "enrich failed: "+err.Error())
	}
	return response.OK(c, result)
}

// EnrichUnrecognisedAll godoc
// @Summary     Re-parse unrecognised emails for all users via LLM
// @Description Internal endpoint called by the worker cron to enrich emails
//
//	that failed regex parsing for every connected user.
//
// @Tags        internal
// @Produce     json
// @Param       X-Internal-Key header string true "Internal shared secret"
// @Success     200 {object} map[string]interface{}
// @Failure     401 {object} map[string]interface{}
// @Router      /internal/gmail/enrich-all [post]
func (h *GmailHandler) EnrichUnrecognisedAll(c *fiber.Ctx) error {
	result := h.svc.EnrichUnrecognisedAll(c.Context())
	return response.OK(c, result)
}

// Logs godoc
// @Summary     Gmail ingestion history
// @Description Returns a paginated list of emails we have processed (or attempted).
// @Tags        gmail
// @Produce     json
// @Security    BearerAuth
// @Param       limit  query int false "Page size (default 50)"
// @Param       offset query int false "Page offset (default 0)"
// @Success     200 {object} dto.LogsPage
// @Failure     401 {object} map[string]interface{}
// @Router      /gmail/logs [get]
func (h *GmailHandler) Logs(c *fiber.Ctx) error {
	userID := middleware.CurrentUserID(c)
	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)
	if limit < 1 || limit > 100 {
		limit = 50
	}

	logs, total, err := h.svc.Logs(c.Context(), userID, limit, offset)
	if err != nil {
		return response.InternalError(c, "could not fetch logs")
	}
	return response.OK(c, dto.LogsPage{Logs: logs, Total: total, Limit: limit, Offset: offset})
}
