package http

import (
	"context"
	"encoding/json"
	"time"

	"github.com/perunio/perunio-facturador/internal/auth"
	"github.com/perunio/perunio-facturador/internal/db"
	"github.com/perunio/perunio-facturador/internal/scheduler"
)

const (
	// tickInterval is how often due schedules are polled. Schedules are configured
	// to the minute, so a finer interval buys nothing.
	tickInterval = 1 * time.Minute

	// maxSchedulesPerTick bounds the work of a single tick so one tenant with many
	// due schedules can't monopolize a run or hold the claim transaction open.
	maxSchedulesPerTick = 25

	// maxIssueAttempts caps retries of a *transient* issue failure within one run.
	// A SUNAT rejection is never retried, so this only ever applies to network and
	// timeout errors.
	maxIssueAttempts = 3

	// retryBackoff is the wait between transient issue retries.
	retryBackoff = 5 * time.Second
)

// scheduleFormData is the jsonb payload the frontend stores on a schedule.
//
// It carries the built emission request rather than raw form values so this
// service never re-derives money: `request` is byte-for-byte what the browser
// would POST to /documents/{companyId} for a manual emission, IGV, detracción and
// all. `values` is the form snapshot the UI rehydrates its editor from; the two are
// always written together from a single source, so they cannot drift.
type scheduleFormData struct {
	Request createDocumentBody `json:"request"`
}

// StartScheduler runs the comprobantes recurrentes/programados ticker until ctx is
// cancelled. Safe to run on several replicas: claiming is transactional and the
// run-row unique constraint is the at-most-once guarantee.
//
// No-op when the scheduler is disabled or no admin pool was configured.
func (s *Server) StartScheduler(ctx context.Context) {
	if !s.cfg.SchedulerEnabled || s.adminPool == nil {
		s.log.Info("comprobante scheduler disabled")
		return
	}

	s.log.Info("comprobante scheduler started", "interval", tickInterval.String())
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("comprobante scheduler stopped")
			return
		case <-ticker.C:
			s.runTick(ctx)
		}
	}
}

// runTick claims and executes every schedule due right now.
func (s *Server) runTick(ctx context.Context) {
	// Claiming uses the admin (BYPASSRLS) pool: a background tick has no current
	// tenant, so the RLS-bound app role would see no rows at all.
	due, err := s.adminPool.ClaimDueSchedules(ctx, time.Now(), maxSchedulesPerTick)
	if err != nil {
		s.log.Error("claim due schedules", "error", err)
		return
	}
	if len(due) == 0 {
		return
	}

	s.log.Info("executing due comprobante schedules", "count", len(due))
	for _, sched := range due {
		// Per-schedule guard so one tenant's failure can't abort the batch.
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					s.log.Error("panic executing schedule", "recover", rec, "scheduleId", sched.ID, "kind", sched.Kind)
				}
			}()
			s.executeSchedule(ctx, sched)
		}()
	}
}

// executeSchedule emits one due schedule and records the outcome.
func (s *Server) executeSchedule(ctx context.Context, sched db.DueSchedule) {
	// Synthesize the tenant context the DB layer and pipeline expect. Every helper
	// resolves the tenant from the context rather than from a token, so this is all
	// it takes for a background run to reuse the entire HTTP emission path.
	runCtx := auth.WithUser(ctx, &auth.JWTPayload{
		UserID:   sched.UserID,
		TenantID: sched.TenantID,
	})

	doc, attempt, pErr := s.emitSchedule(runCtx, sched)

	outcome := db.ScheduleRunOutcome{Attempt: attempt}
	emitted := false

	switch {
	case pErr != nil:
		outcome.Status = "failed"
		code := pErr.code
		msg := pErr.message
		outcome.ErrorCode = &code
		outcome.ErrorMessage = &msg
		s.log.Error("schedule emission failed",
			"scheduleId", sched.ID, "kind", sched.Kind, "code", pErr.code, "error", pErr.message)

	case doc != nil && doc.Status == "rejected":
		// The pipeline ran to completion and SUNAT refused the document. That is a
		// terminal business outcome, not a technical failure: the correlative is
		// spent and a retry would be rejected identically.
		outcome.Status = "rejected"
		outcome.DocumentID = &doc.ID
		code := "SUNAT_REJECTED"
		if doc.SunatResponseCode != nil {
			code = *doc.SunatResponseCode
		}
		msg := "SUNAT rechazó el comprobante"
		if doc.SunatResponseDescription != nil {
			msg = *doc.SunatResponseDescription
		}
		outcome.ErrorCode = &code
		outcome.ErrorMessage = &msg

	default:
		outcome.Status = "success"
		outcome.DocumentID = &doc.ID
		emitted = true
	}

	if err := s.adminPool.CompleteScheduleRun(ctx, sched.RunID, outcome); err != nil {
		s.log.Error("complete schedule run", "error", err, "runId", sched.RunID)
	}

	// Advance the schedule regardless of the outcome — a failed period must not be
	// retried forever, and a recurring schedule stays armed for its next slot.
	switch sched.Kind {
	case db.ScheduleKindRecurrente:
		s.advanceRecurrente(ctx, sched, emitted)
	case db.ScheduleKindProgramado:
		estado := "emitido"
		if !emitted {
			estado = "fallido"
		}
		if err := s.adminPool.SetProgramadoEstado(ctx, sched.ID, estado); err != nil {
			s.log.Error("set programado estado", "error", err, "programadoId", sched.ID)
		}
	}
}

// emitSchedule creates the draft once, then issues it, retrying only failures that
// a later attempt could plausibly survive.
//
// The draft is deliberately created outside the retry loop: it reserves a
// correlative, so re-creating it per attempt would burn a correlative each time and
// leave orphaned drafts behind.
func (s *Server) emitSchedule(ctx context.Context, sched db.DueSchedule) (*docResult, int, *pipelineError) {
	var payload scheduleFormData
	if err := json.Unmarshal(sched.FormData, &payload); err != nil {
		s.log.Error("unmarshal schedule form data", "error", err, "scheduleId", sched.ID)
		return nil, 1, newPipelineError(400, "INVALID_FORM_DATA", "Los datos guardados de la programación no son válidos")
	}

	body := payload.Request
	// The stored snapshot carries the date it was created on. The comprobante must
	// be dated when it is actually emitted, or SUNAT sees a back-dated document.
	nowPeru, err := peruNow()
	if err != nil {
		s.log.Error("resolve peru time", "error", err)
		return nil, 1, newPipelineError(500, "INTERNAL_ERROR", "Error interno del servidor")
	}
	body.IssueDate = nowPeru.Format("2006-01-02")
	issueTime := nowPeru.Format("15:04:05")
	body.IssueTime = &issueTime

	if msg := body.validate(); msg != "" {
		return nil, 1, newPipelineError(400, "VALIDATION_ERROR", msg)
	}

	draft, pErr := s.createDocumentDraft(ctx, sched.CompanyID, body.toInput())
	if pErr != nil {
		return nil, 1, pErr
	}

	attempt := 1
	for {
		// envOverride is empty so the company's current sunat_environment decides —
		// a company in beta emits beta-stamped documents on its own correlative
		// sequence, exactly as a manual emission would.
		updated, pErr := s.runIssuePipeline(ctx, sched.CompanyID, draft.ID, "")
		if pErr == nil {
			return &docResult{
				ID:                       updated.ID,
				Status:                   updated.Status,
				SunatResponseCode:        updated.SunatResponseCode,
				SunatResponseDescription: updated.SunatResponseDescription,
			}, attempt, nil
		}
		if !pErr.retryable || attempt >= maxIssueAttempts {
			return nil, attempt, pErr
		}

		s.log.Warn("retrying transient schedule emission failure",
			"scheduleId", sched.ID, "docId", draft.ID, "attempt", attempt, "code", pErr.code)
		attempt++

		select {
		case <-ctx.Done():
			return nil, attempt, pErr
		case <-time.After(retryBackoff):
		}
	}
}

// docResult is the slice of the issued document the scheduler records.
type docResult struct {
	ID                       string
	Status                   string
	SunatResponseCode        *string
	SunatResponseDescription *string
}

// advanceRecurrente moves a recurrente to its next slot, retiring it when an end
// condition is reached.
func (s *Server) advanceRecurrente(ctx context.Context, sched db.DueSchedule, emitted bool) {
	rule := scheduler.RecurrenceRule{
		Frecuencia:  scheduler.Frecuencia(sched.Frecuencia),
		DiaMes:      sched.DiaMes,
		DiaSemana:   sched.DiaSemana,
		HoraEmision: sched.HoraEmision,
	}

	// Advance from the slot that just ran, not from now, so a late tick can't skip
	// a period.
	next, err := scheduler.NextRunAt(sched.ScheduledFor, rule)
	if err != nil {
		s.log.Error("calculate next run", "error", err, "recurrenteId", sched.ID)
		return
	}

	periodosEmitidos := sched.PeriodosEmitidos
	if emitted {
		periodosEmitidos++
	}

	var nextRunAt *time.Time
	if !scheduler.IsExhausted(sched.CantidadPeriodos, periodosEmitidos, sched.FechaFin, next) {
		nextRunAt = &next
	}

	if err := s.adminPool.AdvanceRecurrente(ctx, sched.ID, nextRunAt, emitted); err != nil {
		s.log.Error("advance recurrente", "error", err, "recurrenteId", sched.ID)
	}
}

// peruNow returns the current instant in Peru's wall clock.
func peruNow() (time.Time, error) {
	loc, err := time.LoadLocation("America/Lima")
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().In(loc), nil
}
