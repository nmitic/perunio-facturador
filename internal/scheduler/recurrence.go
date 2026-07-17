package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	// Embeds the IANA tz database in the binary. The runtime image is alpine-based
	// and carries no tzdata, so without this LoadLocation("America/Lima") fails and
	// every schedule would fire at the wrong hour.
	_ "time/tzdata"
)

// peruTimezone is the wall-clock zone every schedule is expressed in. Peru has no
// DST, but the offset is still applied through the location rather than hardcoded.
const peruTimezone = "America/Lima"

// Frecuencia mirrors the comprobante_frecuencia enum in perunio-backend.
type Frecuencia string

const (
	FrecuenciaDiaria  Frecuencia = "diaria"
	FrecuenciaSemanal Frecuencia = "semanal"
	FrecuenciaMensual Frecuencia = "mensual"
)

// RecurrenceRule is the timing rule of a comprobante recurrente.
//
// This is the Go twin of calculateNextRunAt in
// perunio-backend/src/utils/comprobante-schedule.ts: Node derives the *first* run
// when the schedule is created, and this side advances it after every run. The two
// must agree, which is what recurrence_test.go pins down — its expectations are
// deliberately identical to the TypeScript suite's.
type RecurrenceRule struct {
	Frecuencia  Frecuencia
	DiaMes      *int // 1-31, mensual only
	DiaSemana   *int // 0=Sunday..6=Saturday, semanal only
	HoraEmision string
}

func peruLocation() (*time.Location, error) {
	return time.LoadLocation(peruTimezone)
}

// parseTimeOfDay splits "HH:mm".
func parseTimeOfDay(timeOfDay string) (hours, minutes int, err error) {
	parts := strings.Split(timeOfDay, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time format: %s", timeOfDay)
	}
	hours, err = strconv.Atoi(parts[0])
	if err != nil || hours < 0 || hours > 23 {
		return 0, 0, fmt.Errorf("invalid time format: %s", timeOfDay)
	}
	minutes, err = strconv.Atoi(parts[1])
	if err != nil || minutes < 0 || minutes > 59 {
		return 0, 0, fmt.Errorf("invalid time format: %s", timeOfDay)
	}
	return hours, minutes, nil
}

// atPeruTime rebuilds an instant from a Peru-local calendar day plus "HH:mm".
func atPeruTime(peruDay time.Time, timeOfDay string, loc *time.Location) (time.Time, error) {
	hours, minutes, err := parseTimeOfDay(timeOfDay)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(peruDay.Year(), peruDay.Month(), peruDay.Day(), hours, minutes, 0, 0, loc), nil
}

// daysInMonth returns the number of days in the month containing t.
func daysInMonth(t time.Time) int {
	// Day 0 of the next month is the last day of this one.
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}

// NextRunAt returns the run instant after base for the given rule.
//
// It always advances from base — the period that just ran — never from "now", so a
// late tick or a retry can't cause a period to be skipped. The monthly and weekly
// cases re-derive their slot from DiaMes/DiaSemana rather than offsetting base, so
// a manual "emitir ahora" on an arbitrary day doesn't shift the cadence.
func NextRunAt(base time.Time, rule RecurrenceRule) (time.Time, error) {
	loc, err := peruLocation()
	if err != nil {
		return time.Time{}, fmt.Errorf("load peru timezone: %w", err)
	}
	basePeru := base.In(loc)

	switch rule.Frecuencia {
	case FrecuenciaDiaria:
		return atPeruTime(basePeru.AddDate(0, 0, 1), rule.HoraEmision, loc)

	case FrecuenciaSemanal:
		target := int(basePeru.Weekday())
		if rule.DiaSemana != nil {
			target = *rule.DiaSemana
		}
		delta := ((target - int(basePeru.Weekday())) + 7) % 7
		if delta == 0 {
			delta = 7 // same weekday means a full week away, not today
		}
		return atPeruTime(basePeru.AddDate(0, 0, delta), rule.HoraEmision, loc)

	case FrecuenciaMensual:
		day := basePeru.Day()
		if rule.DiaMes != nil {
			day = *rule.DiaMes
		}
		// First day of the month after base's month.
		firstOfNextMonth := time.Date(basePeru.Year(), basePeru.Month()+1, 1, 0, 0, 0, 0, loc)
		// Clamp so a day-31 schedule still fires in short months (31 -> 28/29 Feb).
		if max := daysInMonth(firstOfNextMonth); day > max {
			day = max
		}
		return atPeruTime(time.Date(firstOfNextMonth.Year(), firstOfNextMonth.Month(), day, 0, 0, 0, 0, loc), rule.HoraEmision, loc)

	default:
		return time.Time{}, fmt.Errorf("unknown frecuencia: %s", rule.Frecuencia)
	}
}

// IsExhausted reports whether a recurrente has reached either end condition.
// periodosEmitidos must already include the run that just completed.
func IsExhausted(cantidadPeriodos *int, periodosEmitidos int, fechaFin *time.Time, nextRunAt time.Time) bool {
	if cantidadPeriodos != nil && periodosEmitidos >= *cantidadPeriodos {
		return true
	}
	if fechaFin != nil {
		loc, err := peruLocation()
		if err != nil {
			return false
		}
		// fechaFin is an inclusive Peru-local calendar day.
		endOfDay := time.Date(fechaFin.Year(), fechaFin.Month(), fechaFin.Day(), 23, 59, 59, 0, loc)
		if nextRunAt.After(endOfDay) {
			return true
		}
	}
	return false
}
