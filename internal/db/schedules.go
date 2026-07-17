package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/perunio/perunio-facturador/internal/model"
)

// ScheduleKind distinguishes the two schedule tables a run can belong to.
//
// Alias rather than a second declaration: "which schedule kind" and "what a
// comprobante's origin is" are the same vocabulary, and duplicating the constants
// would let the scheduler and the historial drift apart silently.
type ScheduleKind = model.ScheduleOrigin

const (
	ScheduleKindRecurrente = model.ScheduleOriginRecurrente
	ScheduleKindProgramado = model.ScheduleOriginProgramado
)

// DueSchedule is a schedule the scheduler has claimed for execution, joined with
// everything the run needs: the tenant owner (to synthesize the auth context) and
// the form snapshot to emit from.
type DueSchedule struct {
	Kind      ScheduleKind
	ID        string
	TenantID  string
	CompanyID string
	// UserID is the tenant owner. Background runs have no logged-in user, so this
	// is what the synthesized JWT payload carries.
	UserID       string
	Tipo         string
	FormData     []byte
	ScheduledFor time.Time

	// Recurrence fields, set for ScheduleKindRecurrente only.
	Frecuencia       string
	DiaMes           *int
	DiaSemana        *int
	HoraEmision      string
	CantidadPeriodos *int
	PeriodosEmitidos int
	FechaFin         *time.Time

	// RunID is the comprobante_schedule_runs row claimed for this execution.
	RunID string
}

// staleRunAfter is how long a run may sit in 'running' before another tick may
// adopt it. Longer than any plausible SUNAT round trip, so a slow run is never
// stolen from a healthy process; short enough that a crashed process doesn't strand
// the period forever.
const staleRunAfter = 15 * time.Minute

// ClaimDueSchedules finds schedules due at or before `now` and claims each by
// inserting its comprobante_schedule_runs row inside the same transaction.
//
// Runs cross-tenant, so it must be called on a BYPASSRLS (perunio_admin) pool —
// the RLS-bound app role sees no rows without a current tenant.
//
// Claiming is what makes concurrent replicas safe. Two mechanisms combine:
//   - FOR UPDATE SKIP LOCKED, so two ticks don't pick the same row; and
//   - the UNIQUE (schedule, scheduled_for) constraint on the run row, which is the
//     actual at-most-once guarantee — a replica that loses the race gets a conflict
//     and skips instead of emitting a duplicate invoice.
//
// A run left 'running' by a crashed process is adopted once it goes stale, so the
// period is retried rather than stranded.
func (p *Pool) ClaimDueSchedules(ctx context.Context, now time.Time, limit int) ([]DueSchedule, error) {
	var claimed []DueSchedule

	err := pgx.BeginTxFunc(ctx, p.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		recurrentes, err := claimDueRecurrentes(ctx, tx, now, limit)
		if err != nil {
			return err
		}
		claimed = append(claimed, recurrentes...)

		remaining := limit - len(claimed)
		if remaining <= 0 {
			return nil
		}

		programados, err := claimDueProgramados(ctx, tx, now, remaining)
		if err != nil {
			return err
		}
		claimed = append(claimed, programados...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func claimDueRecurrentes(ctx context.Context, tx pgx.Tx, now time.Time, limit int) ([]DueSchedule, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.tenant_id, r.company_id, t.user_id, r.tipo::text, r.form_data,
		       r.next_run_at, r.frecuencia::text, r.dia_mes, r.dia_semana, r.hora_emision,
		       r.cantidad_periodos, r.periodos_emitidos, r.fecha_fin
		FROM comprobantes_recurrentes r
		JOIN tenants t ON t.id = r.tenant_id
		WHERE r.activo = true
		  AND r.next_run_at IS NOT NULL
		  AND r.next_run_at <= $1
		ORDER BY r.next_run_at
		LIMIT $2
		FOR UPDATE OF r SKIP LOCKED`, now, limit)
	if err != nil {
		return nil, err
	}

	candidates := make([]DueSchedule, 0, limit)
	for rows.Next() {
		s := DueSchedule{Kind: ScheduleKindRecurrente}
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.CompanyID, &s.UserID, &s.Tipo, &s.FormData,
			&s.ScheduledFor, &s.Frecuencia, &s.DiaMes, &s.DiaSemana, &s.HoraEmision,
			&s.CantidadPeriodos, &s.PeriodosEmitidos, &s.FechaFin,
		); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	claimed := make([]DueSchedule, 0, len(candidates))
	for _, s := range candidates {
		runID, err := claimRun(ctx, tx, "recurrente_id", s.ID, s.TenantID, s.ScheduledFor)
		if err != nil {
			return nil, err
		}
		if runID == "" {
			continue // another replica owns this period
		}
		s.RunID = runID
		claimed = append(claimed, s)
	}
	return claimed, nil
}

func claimDueProgramados(ctx context.Context, tx pgx.Tx, now time.Time, limit int) ([]DueSchedule, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.tenant_id, p.company_id, t.user_id, p.tipo::text, p.form_data, p.run_at
		FROM comprobantes_programados p
		JOIN tenants t ON t.id = p.tenant_id
		WHERE p.estado = 'pendiente'
		  AND p.run_at <= $1
		ORDER BY p.run_at
		LIMIT $2
		FOR UPDATE OF p SKIP LOCKED`, now, limit)
	if err != nil {
		return nil, err
	}

	candidates := make([]DueSchedule, 0, limit)
	for rows.Next() {
		s := DueSchedule{Kind: ScheduleKindProgramado}
		if err := rows.Scan(&s.ID, &s.TenantID, &s.CompanyID, &s.UserID, &s.Tipo, &s.FormData, &s.ScheduledFor); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	claimed := make([]DueSchedule, 0, len(candidates))
	for _, s := range candidates {
		runID, err := claimRun(ctx, tx, "programado_id", s.ID, s.TenantID, s.ScheduledFor)
		if err != nil {
			return nil, err
		}
		if runID == "" {
			continue
		}
		s.RunID = runID
		claimed = append(claimed, s)
	}
	return claimed, nil
}

// claimRun inserts the run row for (schedule, scheduledFor), returning its id.
// Returns "" when the period is already owned by a live run elsewhere.
func claimRun(ctx context.Context, tx pgx.Tx, parentColumn, scheduleID, tenantID string, scheduledFor time.Time) (string, error) {
	var runID string

	// The unique constraint on (schedule, scheduled_for) is what makes this safe:
	// a losing replica conflicts here instead of emitting a second comprobante.
	err := tx.QueryRow(ctx, `
		INSERT INTO comprobante_schedule_runs (tenant_id, `+parentColumn+`, scheduled_for, status)
		VALUES ($1, $2, $3, 'running')
		ON CONFLICT DO NOTHING
		RETURNING id`, tenantID, scheduleID, scheduledFor).Scan(&runID)
	if err == nil {
		return runID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	// Conflict: a run row already exists for this period. Adopt it only if it was
	// left 'running' by a process that died — a finished run means the period is
	// done, and a live run must not be disturbed.
	//
	// The staleness window is passed as seconds and multiplied into an interval:
	// Go's Duration.String() ("15m0s") happens to parse as a Postgres interval, but
	// that's a coincidence between two unrelated formats, not a contract.
	err = tx.QueryRow(ctx, `
		UPDATE comprobante_schedule_runs
		SET started_at = now(), attempt = attempt + 1
		WHERE `+parentColumn+` = $1
		  AND scheduled_for = $2
		  AND status = 'running'
		  AND started_at < now() - ($3 * interval '1 second')
		RETURNING id`, scheduleID, scheduledFor, staleRunAfter.Seconds()).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return runID, nil
}

// ScheduleRunOutcome is the terminal state of a run.
type ScheduleRunOutcome struct {
	Status       string // 'success' | 'rejected' | 'failed' | 'skipped'
	DocumentID   *string
	ErrorCode    *string
	ErrorMessage *string
	Attempt      int
}

// CompleteScheduleRun writes a run's terminal state.
func (p *Pool) CompleteScheduleRun(ctx context.Context, runID string, out ScheduleRunOutcome) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE comprobante_schedule_runs
		SET status = $2, document_id = $3, error_code = $4, error_message = $5,
		    attempt = $6, finished_at = now()
		WHERE id = $1`,
		runID, out.Status, out.DocumentID, out.ErrorCode, out.ErrorMessage, out.Attempt)
	return err
}

// AdvanceRecurrente moves a recurrente to its next slot after a run.
//
// nextRunAt nil retires the schedule (an end condition was reached); the row stays
// so its history remains readable. periodosEmitidos is only incremented on a
// successful emission, so a failing schedule doesn't burn through cantidadPeriodos.
func (p *Pool) AdvanceRecurrente(ctx context.Context, id string, nextRunAt *time.Time, emitted bool) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE comprobantes_recurrentes
		SET next_run_at = $2,
		    last_run_at = now(),
		    periodos_emitidos = periodos_emitidos + CASE WHEN $3 THEN 1 ELSE 0 END,
		    updated_at = now()
		WHERE id = $1`, id, nextRunAt, emitted)
	return err
}

// SetProgramadoEstado writes the terminal state of a one-shot programado.
func (p *Pool) SetProgramadoEstado(ctx context.Context, id, estado string) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE comprobantes_programados
		SET estado = $2, updated_at = now()
		WHERE id = $1`, id, estado)
	return err
}
