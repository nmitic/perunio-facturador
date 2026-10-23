package model

import (
	"math"
	"strconv"

	"github.com/nmitic/perunio-sunat-catalogs/sunat"
)

// IssueRequest is the payload the backend sends to issue a document.
type IssueRequest struct {
	// Supplier
	SupplierRUC       string `json:"supplierRuc"`
	SupplierName      string `json:"supplierName"`
	SupplierAddress   string `json:"supplierAddress"`
	EstablishmentCode string `json:"establishmentCode"` // "0000" default

	// Document identity
	DocType     string `json:"docType"` // "01","03","07","08"
	Series      string `json:"series"`  // "F001","B001","FC01","FD01"
	Correlative int    `json:"correlative"`

	// Dates
	IssueDate string `json:"issueDate"` // YYYY-MM-DD
	IssueTime string `json:"issueTime"` // HH:mm:ss

	// Currency & operation
	CurrencyCode string `json:"currencyCode"` // ISO 4217
	// ExchangeRate is the tipo de cambio to soles for foreign-currency documents
	// (decimal as string); empty for PEN. Carried for PLE/SIRE and detracción
	// records; it does not change the XML monetary amounts (the document is
	// emitted in its own currency, and the detracción monto arrives already in PEN).
	ExchangeRate  string `json:"exchangeRate,omitempty"`
	OperationType string `json:"operationType"` // Cat.51

	// Customer
	CustomerDocType   string `json:"customerDocType"` // Cat.06
	CustomerDocNumber string `json:"customerDocNumber"`
	CustomerName      string `json:"customerName"`
	CustomerAddress   string `json:"customerAddress"`

	// Totals (strings to preserve decimal precision)
	Subtotal           string `json:"subtotal"`
	TotalIGV           string `json:"totalIgv"`
	TotalISC           string `json:"totalIsc"`
	TotalOtherTaxes    string `json:"totalOtherTaxes"`
	TotalDiscount      string `json:"totalDiscount"`
	TotalAmount        string `json:"totalAmount"`
	TaxInclusiveAmount string `json:"taxInclusiveAmount"`

	// GlobalDiscount is a document-level descuento global (Cat.53 code 02 —
	// afecta la base imponible del IGV). Empty/"0" = none. It is emitted as a
	// document-level cac:AllowanceCharge that reduces the gravado IGV base and
	// valor de venta. Valid on facturas/boletas and on notas de crédito
	// (motivo 04, descuento global sobre el comprobante referenciado).
	GlobalDiscount string `json:"globalDiscount"`

	// Forma de pago (Cat. SUNAT). Empty defaults to "contado" downstream.
	FormaPago string         `json:"formaPago"`
	Cuotas    []CuotaCredito `json:"cuotas,omitempty"`

	// Anticipos are the prior advance-payment comprobantes this document
	// (a factura de regularización) applies. Each one emits a
	// cac:AdditionalDocumentReference + cac:PrepaidPayment pair and a document
	// cac:AllowanceCharge (Cat.53 "04") that reduces the gravado IGV base by
	// its BaseAmount, and their sum is declared as cbc:PrepaidAmount.
	// Subtotal, TotalIGV and TaxInclusiveAmount arrive GROSS (the full sale);
	// only TotalAmount, the payable, is net of the anticipos.
	// Empty for every ordinary document.
	Anticipos []Anticipo `json:"anticipos,omitempty"`

	// Detraccion is the SPOT (Sistema de Pago de Obligaciones Tributarias)
	// withholding. nil = the operation is not subject to detracción. When set,
	// OperationType must be 1001-1004 (or 0104 for an anticipo) and the document
	// emits the detracción cac:PaymentMeans + cac:PaymentTerms + legend 2006. It
	// does not alter the document totals — it is a payment instruction, not a
	// discount. The rules are written up in DETRACCIONES.md.
	Detraccion *Detraccion `json:"detraccion,omitempty"`

	// MontoOperacion is the monto acordado of the venta con anticipos this
	// comprobante belongs to, in the document's currency; empty otherwise.
	// Perunio measures the SPOT umbral of a venta con anticipos against the whole
	// operación — its own conservative reading, decided without a SUNAT source
	// (DETRACCIONES.md › Cuándo aplica, ◇) — so validateDetraccion takes the
	// greater of this and TaxInclusiveAmount. The pipeline fills it from the
	// venta row — never from the client.
	MontoOperacion string `json:"-"`

	// IGVReducido: the issuer applies the IGV reducido of 10.5 % (Ley 31556,
	// MYPE restaurantes, hoteles y alojamientos turísticos in padrón 01), so
	// every gravado onerosa line is taxed at 10.5 % instead of 18 %. The pipeline
	// fills it from the company settings, never from the client. SUNAT accepts
	// either rate (3103) but one per document (3462), and observes 4439 when the
	// issuer is not in the padrón.
	IGVReducido bool `json:"-"`

	// AdquirenteSectorPublico: the buyer is a Sector Público Nacional entity
	// (art. 18 a) LIR). The exclusion of a comprobante without crédito fiscal
	// (Anexos 2 and 3, R.S. 183-2004 arts. 8 and 13; transporte de carga, R.S.
	// 073-2006 art. 3) "no opera" for one, so a boleta to it still carries the
	// detracción. The user answers it at emission; the XML does not carry it.
	AdquirenteSectorPublico bool `json:"adquirenteSectorPublico,omitempty"`

	// Anexo1UnidadTransporte: the Anexo 1 bienes on one transport unit add up to
	// more than ½ UIT, so the operación is subject even at or under the umbral
	// (R.S. 183-2004 art. 2.1). The user answers it at emission.
	Anexo1UnidadTransporte bool `json:"anexo1UnidadTransporte,omitempty"`

	// TransporteCarga carries the extra data SUNAT demands when the detracción
	// código is 027 (servicio de transporte de carga), which declares tipo de
	// operación 1004. Nil for every other comprobante. See buildLineDelivery.
	TransporteCarga *TransporteCarga `json:"transporteCarga,omitempty"`

	// Hidrobiologicos carries the embarcación and descarga data a detracción de
	// recursos hidrobiológicos (Cat.54 004 / tipo de operación 1002) declares on
	// every line, next to each line's EspecieVendida and CantidadEspecie.
	Hidrobiologicos *Hidrobiologicos `json:"hidrobiologicos,omitempty"`

	// Notes
	Notes []Note `json:"notes"`

	// Credit/Debit note reference
	ReferenceDocType        string `json:"referenceDocType"`
	ReferenceDocSeries      string `json:"referenceDocSeries"`
	ReferenceDocCorrelative int    `json:"referenceDocCorrelative"`
	ReasonCode              string `json:"reasonCode"`
	ReasonDescription       string `json:"reasonDescription"`

	// ReferencedTotal is the importe total of the comprobante a nota de crédito
	// modifies, for SUNAT 3286. The pipeline fills it from that comprobante's
	// row, never from the client; nil when Perunio did not issue it (its total
	// is then unknown) or for any other document.
	ReferencedTotal *ReferencedTotal `json:"-"`

	// Items
	Items []LineItem `json:"items"`

	// Certificate
	CertificateURL      string `json:"certificateUrl"`      // Presigned R2 URL
	CertificatePassword string `json:"certificatePassword"` // AES-256-GCM encrypted

	// SUNAT credentials
	SunatUsername string `json:"sunatUsername"`
	SunatPassword string `json:"sunatPassword"`
	Environment   string `json:"environment"` // "beta" or "production"
}

// DetraccionEnMoneda returns the detracción converted into the document's own
// currency, and whether the conversion was possible.
//
// The monto is always declared in soles (SUNAT liquidates the SPOT in PEN), so a
// USD or EUR document has to divide by its tipo de cambio to state anything about
// the detracción in the currency the rest of the comprobante is written in. With
// no usable rate the caller gets ok=false and must not guess — validateDetraccion
// requires the exchangeRate precisely so this never happens on a real emission.
func (r IssueRequest) DetraccionEnMoneda() (float64, bool) {
	if r.Detraccion == nil {
		return 0, false
	}
	monto, err := strconv.ParseFloat(r.Detraccion.Monto, 64)
	if err != nil {
		return 0, false
	}
	if r.CurrencyCode == "" || r.CurrencyCode == "PEN" {
		return monto, true
	}
	rate, err := strconv.ParseFloat(r.ExchangeRate, 64)
	if err != nil || rate <= 0 {
		return 0, false
	}
	return math.Round(monto/rate*100) / 100, true
}

// DetraccionDepositaEmisor reports whether the seller, not the buyer, is the
// party obligated to make the detracción deposit from the start. Perunio can
// emit one such case: a venta de inmueble (Cat.54 040) on a comprobante that
// grants neither crédito fiscal nor gasto/costo — a boleta, or a nota that
// corrects one (R.S. 183-2004 art. 10.3 b.i, added by R.S. 022-2013). The
// seller's other cases (paid in full without the deposit; Anexo 1 transport)
// depend on what happens after emission, so the comprobante cannot know them.
func (r IssueRequest) DetraccionDepositaEmisor() bool {
	if r.Detraccion == nil {
		return false
	}
	// An Anexo 1 operación under its umbral that applies only because one
	// transport unit carries more than ½ UIT (art. 2.1): the proveedor carries
	// and delivers the bienes, so it deposits (R.S. 183-2004 art. 5.1 b.1).
	// validateDetraccionCatalogo refuses the answer on an operación above the
	// umbral, where the buyer deposits as usual.
	if r.Anexo1UnidadTransporte && sunat.Cat54Anexo(r.Detraccion.Codigo) == "1" {
		return true
	}
	if sunat.Cat54Anexo(r.Detraccion.Codigo) != "inmueble" {
		return false
	}
	isNota := r.DocType == sunat.Cat01NotaCredito || r.DocType == sunat.Cat01NotaDebito
	return r.DocType == sunat.Cat01Boleta || (isNota && r.ReferenceDocType == sunat.Cat01Boleta)
}

// NetoPendientePago is the "monto neto pendiente de pago": what the receptor
// actually transfers to the emisor, in the document's own currency.
//
// The receptor splits their payment in two: the detracción goes into the
// EMISOR'S OWN cuenta de detracciones at the Banco de la Nación — a restricted
// account they may only draw on to pay their own taxes — and only the remainder
// reaches the emisor's operating account. So the detracción is not a tax the
// emisor pays, but it is not money the receptor still owes them either, and it
// comes off the payable total. TotalAmount is cbc:PayableAmount, already net of
// anticipos, which is the other deduction.
//
// It is also why the detracción can never sit inside a cuota: it falls due at
// payment (or the 5th business day of the following month, whichever is first)
// no matter what credit terms the comprobante grants, so it is never financed.
//
// SUNAT keys the forma de pago a crédito on this figure and not on TotalAmount:
// the leading cac:PaymentTerms "Credito" carries it and the cuotas must add up to
// it. Printed representations state it as "Neto a pagar".
//
// Only the deposit the buyer makes comes off (R.S. 193-2020: "el monto del
// depósito que deba efectuar el adquirente o usuario"). When the seller is the
// obligated party (DetraccionDepositaEmisor), the buyer pays it the whole total
// and the seller deposits, so nothing is deducted.
//
// Without detracción it is simply the payable, so callers can use it unguarded.
func (r IssueRequest) NetoPendientePago() float64 {
	total, err := strconv.ParseFloat(r.TotalAmount, 64)
	if err != nil {
		return 0
	}
	if r.DetraccionDepositaEmisor() {
		return total
	}
	monto, ok := r.DetraccionEnMoneda()
	if !ok {
		return total
	}
	if neto := math.Round((total-monto)*100) / 100; neto > 0 {
		return neto
	}
	return 0
}

// IsIGVReducidoRate reports whether igv ÷ base is the IGV reducido (10.5 %)
// rather than the general 18 % — whichever is nearer. A nota reads the rate of
// the comprobante it corrects this way, from that comprobante's own lines.
func IsIGVReducidoRate(igv, base float64) bool {
	if base <= 0 {
		return false
	}
	pct := igv / base * 100
	return math.Abs(pct-10.5) < math.Abs(pct-18)
}

// Anticipo is one prior advance-payment comprobante applied to (deducted
// from) a factura de regularización. DocID is its SERIE-CORRELATIVO
// ("F001-00000042") and DocTypeCode the Cat.12 related-document type
// ("02" factura de anticipo, "03" boleta de anticipo). TotalAmount is the
// anticipo amount WITH IGV, carried by cbc:PaidAmount / cbc:PrepaidAmount and
// deducted from the payable total. BaseAmount is its IGV-exclusive base, which
// the Cat.53 "04" cac:AllowanceCharge deducts from the base imponible — the
// factura de anticipo already declared the IGV on it. Both figures reach the
// XML at different levels; see buildLegalMonetaryTotal. Amounts are decimal
// strings in the document currency.
// SourceDocumentID is the issued_documents row the anticipo was picked from,
// nil when it was entered manually (emitted outside Perunio).
type Anticipo struct {
	DocID            string  `json:"docId"`
	DocTypeCode      string  `json:"docTypeCode"`
	TotalAmount      string  `json:"totalAmount"`
	BaseAmount       string  `json:"baseAmount"`
	SourceDocumentID *string `json:"sourceDocumentId,omitempty"`
}

// CuotaCredito is one installment in a credit-sale invoice.
type CuotaCredito struct {
	Numero           int    `json:"numero"`
	Monto            string `json:"monto"`            // decimal as string
	FechaVencimiento string `json:"fechaVencimiento"` // YYYY-MM-DD
}

// Detraccion is the detracción (SPOT) declared on a document. Codigo is the
// Cat.54 bien/servicio code, Porcentaje the withholding rate and Monto the
// amount (always in PEN, decimal as string). CuentaBN is the supplier's Banco
// de la Nación detracción account; it is resolved at issue time (per-document
// override, else the company default) so the request always carries the
// effective value.
type Detraccion struct {
	Codigo     string `json:"codigo"`     // Cat.54
	Porcentaje string `json:"porcentaje"` // decimal as string
	Monto      string `json:"monto"`      // decimal as string, PEN
	CuentaBN   string `json:"cuentaBN"`   // Banco de la Nación account
}

// TransporteCarga is the per-operation data a detracción de transporte de carga
// (Cat.54 027 / tipo de operación 1004) must declare. SUNAT models it at the
// line level, but the data describes the trip the whole comprobante documents,
// so it is stored once per document and emitted on the FIRST line — the
// validation rules only require one occurrence of each tag.
//
// Every field here is mandatory when present at all (faults 3116-3126):
// omitting any one of them is an outright rejection.
type TransporteCarga struct {
	// Ubigeo (Cat.13, 6 digits) + free-form street of each end of the trip.
	OrigenUbigeo     string `json:"origenUbigeo"`
	OrigenDireccion  string `json:"origenDireccion"`
	DestinoUbigeo    string `json:"destinoUbigeo"`
	DestinoDireccion string `json:"destinoDireccion"`

	// Free-text description of the trip (an..500).
	DetalleViaje string `json:"detalleViaje"`

	// The three valores referenciales, all in PEN. SUNAT computes the SPOT base
	// as the greater of the importe and the valor referencial, so it wants all
	// three declared: the service's own reference value ("01"), the one over the
	// effective load ("02") and the one over the vehicle's nominal payload ("03").
	ValorReferencialServicio      string `json:"valorReferencialServicio"`
	ValorReferencialCargaEfectiva string `json:"valorReferencialCargaEfectiva"`
	ValorReferencialCargaUtil     string `json:"valorReferencialCargaUtil"`
}

// Hidrobiologicos is the per-operation half of the six Cat.55 concepts a
// detracción de recursos hidrobiológicos (Cat.54 004 / tipo de operación 1002)
// must declare. SUNAT checks all six on EVERY line (ERRORs 3063 and 3130-3135
// run in the line template of ValidaExprRegFactura / ValidaExprRegBoleta), so
// these four repeat on each line; the especie and its cantidad are the line's
// own (LineItem.EspecieVendida / CantidadEspecie).
type Hidrobiologicos struct {
	MatriculaEmbarcacion string `json:"matriculaEmbarcacion"` // 3001, an..15
	NombreEmbarcacion    string `json:"nombreEmbarcacion"`    // 3002, an..100
	LugarDescarga        string `json:"lugarDescarga"`        // 3004, an..100
	FechaDescarga        string `json:"fechaDescarga"`        // 3005, YYYY-MM-DD
}

// Note is a legend/note attached to the document.
type Note struct {
	Code string `json:"code"` // Cat.52 legend code
	Text string `json:"text"`
}

// LineItem is a single invoice line.
type LineItem struct {
	LineNumber             int    `json:"lineNumber"`
	Description            string `json:"description"`
	Quantity               string `json:"quantity"`
	UnitCode               string `json:"unitCode"` // UN/ECE rec 20
	UnitPrice              string `json:"unitPrice"`
	UnitPriceWithTax       string `json:"unitPriceWithTax"`
	TaxExemptionReasonCode string `json:"taxExemptionReasonCode"` // Cat.07
	IGVAmount              string `json:"igvAmount"`
	ISCAmount              string `json:"iscAmount"`
	ISCTierRange           string `json:"iscTierRange"` // Cat.08: 01/02/03
	DiscountAmount         string `json:"discountAmount"`
	LineTotal              string `json:"lineTotal"`
	PriceTypeCode          string `json:"priceTypeCode"` // "01" or "02"

	// Otros tributos (Cat.05 9999) on this line: the rate as a percentage
	// ("10.00") and its amount, rate × the line's valor de venta (3109). Empty
	// when the line pays none. Only onerosa lines carry it.
	OtherTaxesRate   string `json:"otherTaxesRate,omitempty"`
	OtherTaxesAmount string `json:"otherTaxesAmount,omitempty"`

	// Detracción de recursos hidrobiológicos (004 / 1002) only: the especie sold
	// on this line (Cat.55 3003, an..150) and its cantidad in toneladas métricas
	// (3006, decimal 12,2). Empty on every other line.
	EspecieVendida  string `json:"especieVendida,omitempty"`
	CantidadEspecie string `json:"cantidadEspecie,omitempty"`
}

// ReferencedTotal is the importe total of a comprobante a nota modifies.
type ReferencedTotal struct {
	Amount       string // its cbc:PayableAmount
	CurrencyCode string // ISO 4217
}

// IssueResponse is returned to the backend after processing.
type IssueResponse struct {
	Success      bool          `json:"success"`
	SignedXML    []byte        `json:"signedXml,omitempty"`
	ZipBytes     []byte        `json:"zipBytes,omitempty"`
	CDRBytes     []byte        `json:"cdrBytes,omitempty"`
	PDFBytes     []byte        `json:"pdfBytes,omitempty"`
	ResponseCode string        `json:"responseCode,omitempty"`
	Description  string        `json:"description,omitempty"`
	Observations []Observation `json:"observations,omitempty"`
	QRData       string        `json:"qrData,omitempty"`
	Filename     string        `json:"filename,omitempty"`
	ErrorCode    string        `json:"errorCode,omitempty"`
	ErrorMessage string        `json:"errorMessage,omitempty"`
}

// Observation is a SUNAT observation (code 4000+).
type Observation struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidateResponse is for dry-run validation.
type ValidateResponse struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// ValidationError carries a SUNAT error code.
type ValidationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field"`
}
