package xmlbuilder

import (
	"fmt"
	"math"
	"strings"

	sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// buildNotes returns a CPE's cbc:Note list: leyenda 1000 (monto en letras)
// generated from the payable total, followed by the caller's other leyendas.
// A caller-supplied 1000 is dropped: 1000 means "monto en letras" and nothing
// else, and the issue pipeline used to put free-text observaciones there,
// which breaks rule 3006 on any multi-line or >200-character note.
func buildNotes(req model.IssueRequest) []noteElement {
	notes := []noteElement{{
		Value:            montoEnLetras(req.TotalAmount, req.CurrencyCode),
		LanguageLocaleID: sunat.Cat52MontoLetras,
	}}
	for _, n := range req.Notes {
		if n.Code == sunat.Cat52MontoLetras {
			continue
		}
		notes = append(notes, noteElement{Value: n.Text, LanguageLocaleID: n.Code})
	}
	return notes
}

// montoEnLetras writes an amount the way leyenda 1000 carries it:
// "MIL CIENTO OCHENTA CON 00/100 SOLES". Words are upper case without accents,
// as SUNAT's own samples write them.
func montoEnLetras(amount, currency string) string {
	cents := int64(math.Round(math.Abs(parseDecimal(amount)) * 100))
	return fmt.Sprintf("%s CON %02d/100 %s", numeroEnLetras(cents/100), cents%100, monedaEnLetras(currency))
}

func monedaEnLetras(currency string) string {
	switch currency {
	case "PEN", "":
		return "SOLES"
	case "USD":
		return "DOLARES AMERICANOS"
	case "EUR":
		return "EUROS"
	default:
		return currency
	}
}

var (
	letrasUnidades = [...]string{
		"CERO", "UNO", "DOS", "TRES", "CUATRO", "CINCO", "SEIS", "SIETE", "OCHO", "NUEVE",
		"DIEZ", "ONCE", "DOCE", "TRECE", "CATORCE", "QUINCE", "DIECISEIS", "DIECISIETE", "DIECIOCHO", "DIECINUEVE",
		"VEINTE", "VEINTIUNO", "VEINTIDOS", "VEINTITRES", "VEINTICUATRO", "VEINTICINCO", "VEINTISEIS", "VEINTISIETE", "VEINTIOCHO", "VEINTINUEVE",
	}
	letrasDecenas  = [...]string{"", "", "", "TREINTA", "CUARENTA", "CINCUENTA", "SESENTA", "SETENTA", "OCHENTA", "NOVENTA"}
	letrasCentenas = [...]string{"", "CIENTO", "DOSCIENTOS", "TRESCIENTOS", "CUATROCIENTOS", "QUINIENTOS", "SEISCIENTOS", "SETECIENTOS", "OCHOCIENTOS", "NOVECIENTOS"}
)

// numeroEnLetras spells a non-negative integer in Spanish.
func numeroEnLetras(n int64) string {
	switch {
	case n < 1000:
		return centenasEnLetras(n)
	case n < 1_000_000:
		miles, resto := n/1000, n%1000
		s := "MIL"
		if miles > 1 {
			s = apocope(centenasEnLetras(miles)) + " MIL"
		}
		if resto > 0 {
			s += " " + centenasEnLetras(resto)
		}
		return s
	default:
		millones, resto := n/1_000_000, n%1_000_000
		s := "UN MILLON"
		if millones > 1 {
			s = apocope(numeroEnLetras(millones)) + " MILLONES"
		}
		if resto > 0 {
			s += " " + numeroEnLetras(resto)
		}
		return s
	}
}

// centenasEnLetras spells 0-999.
func centenasEnLetras(n int64) string {
	if n == 100 {
		return "CIEN"
	}
	c, r := n/100, n%100
	var parts []string
	if c > 0 {
		parts = append(parts, letrasCentenas[c])
	}
	if r > 0 || c == 0 {
		parts = append(parts, decenasEnLetras(r))
	}
	return strings.Join(parts, " ")
}

// decenasEnLetras spells 0-99.
func decenasEnLetras(n int64) string {
	if n < 30 {
		return letrasUnidades[n]
	}
	s := letrasDecenas[n/10]
	if u := n % 10; u > 0 {
		s += " Y " + letrasUnidades[u]
	}
	return s
}

// apocope shortens a trailing UNO before MIL / MILLONES: "VEINTIUN MIL",
// "TREINTA Y UN MILLONES".
func apocope(s string) string {
	if strings.HasSuffix(s, "UNO") {
		return strings.TrimSuffix(s, "UNO") + "UN"
	}
	return s
}
