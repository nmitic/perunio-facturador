package scheduler_test

import (
	"testing"
	"time"

	"maragu.dev/is"

	"github.com/perunio/perunio-facturador/internal/scheduler"
)

// Peru is UTC-5 year-round, so a Peru wall-clock time is always the UTC hour + 5.
//
// These expectations are deliberately identical to the ones in
// perunio-backend/src/__tests__/utils/comprobante-schedule.test.ts. Node computes a
// schedule's first run and this package advances it, so if the two rules ever drift
// apart one of the two suites has to fail.
func mustParse(t *testing.T, iso string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, iso)
	is.NotError(t, err)
	return parsed
}

func ptr(n int) *int {
	return &n
}

func TestNextRunAt(t *testing.T) {
	t.Run("should advance a daily schedule by one day, keeping the Peru wall-clock time", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2026-08-15T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaDiaria, HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2026-08-16T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	t.Run("should advance a daily schedule across a month boundary", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2026-08-31T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaDiaria, HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2026-09-01T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	t.Run("should advance a weekly schedule by seven days when already on the target weekday", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2026-08-15T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaSemanal, DiaSemana: ptr(6), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2026-08-22T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	// "Emitir ahora" runs a schedule on an arbitrary day; the cadence must snap
	// back to the configured slot rather than drift from the manual run.
	t.Run("should return a weekly schedule to its configured weekday after an off-day run", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2026-08-19T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaSemanal, DiaSemana: ptr(6), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2026-08-22T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	t.Run("should advance a monthly schedule by one month, preserving the Peru wall-clock time", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2026-08-15T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaMensual, DiaMes: ptr(15), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2026-09-15T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	t.Run("should return a monthly schedule to its configured day after an off-day run", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2026-08-19T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaMensual, DiaMes: ptr(15), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2026-09-15T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	// The clamping rule: a day-31 schedule must not skip short months.
	t.Run("should clamp day 31 to the last day of a 30-day month", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2026-03-31T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaMensual, DiaMes: ptr(31), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2026-04-30T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	t.Run("should clamp day 31 to 28 in a non-leap February", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2026-01-31T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaMensual, DiaMes: ptr(31), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2026-02-28T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	t.Run("should clamp day 30 to 29 in a leap February", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2028-01-30T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaMensual, DiaMes: ptr(30), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2028-02-29T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	// Clamping must not be sticky: a clamped run returns to the requested day.
	t.Run("should recover the requested day after a clamped month", func(t *testing.T) {
		feb, err := scheduler.NextRunAt(mustParse(t, "2026-01-31T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaMensual, DiaMes: ptr(31), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		mar, err := scheduler.NextRunAt(feb, scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaMensual, DiaMes: ptr(31), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2026-03-31T13:00:00Z", mar.UTC().Format(time.RFC3339))
	})

	// Advancing from the run instant, not "now", is what stops a late tick from
	// skipping a period.
	t.Run("should advance from the base instant even when the base is far in the past", func(t *testing.T) {
		next, err := scheduler.NextRunAt(mustParse(t, "2020-01-10T13:00:00Z"), scheduler.RecurrenceRule{
			Frecuencia: scheduler.FrecuenciaMensual, DiaMes: ptr(10), HoraEmision: "08:00",
		})
		is.NotError(t, err)
		is.Equal(t, "2020-02-10T13:00:00Z", next.UTC().Format(time.RFC3339))
	})

	t.Run("should return an error for an unknown frecuencia", func(t *testing.T) {
		_, err := scheduler.NextRunAt(time.Now(), scheduler.RecurrenceRule{Frecuencia: "anual", HoraEmision: "08:00"})
		is.True(t, err != nil)
	})

	t.Run("should return an error for a malformed hora", func(t *testing.T) {
		_, err := scheduler.NextRunAt(time.Now(), scheduler.RecurrenceRule{Frecuencia: scheduler.FrecuenciaDiaria, HoraEmision: "25:00"})
		is.True(t, err != nil)
	})
}

func TestIsExhausted(t *testing.T) {
	nextRun := mustParse(t, "2026-09-15T13:00:00Z")

	t.Run("should not be exhausted with no end conditions set", func(t *testing.T) {
		is.True(t, !scheduler.IsExhausted(nil, 99, nil, nextRun))
	})

	t.Run("should be exhausted once the requested number of periods was emitted", func(t *testing.T) {
		is.True(t, scheduler.IsExhausted(ptr(3), 3, nil, nextRun))
	})

	t.Run("should not be exhausted while periods remain", func(t *testing.T) {
		is.True(t, !scheduler.IsExhausted(ptr(3), 2, nil, nextRun))
	})

	t.Run("should be exhausted when the next run would fall past fechaFin", func(t *testing.T) {
		fin := mustParse(t, "2026-09-01T00:00:00Z")
		is.True(t, scheduler.IsExhausted(nil, 1, &fin, nextRun))
	})

	t.Run("should treat fechaFin as an inclusive Peru-local day", func(t *testing.T) {
		// The next run is 15 Sep 08:00 Peru and fechaFin is 15 Sep — it must run.
		fin := mustParse(t, "2026-09-15T00:00:00Z")
		is.True(t, !scheduler.IsExhausted(nil, 1, &fin, nextRun))
	})
}
