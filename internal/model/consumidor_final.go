package model

// Consumidor final is the anonymous buyer a boleta falls back to when no
// customer is identified: Cat.06 doc type "0" (sunat.Cat06DocTribNoDomSinRUC)
// with this número and name. Boletas over S/700 may not use it — they require a
// real DNI or RUC.
//
// This is a Perunio default, not a SUNAT catálogo, which is why it stayed behind
// when catalog.go was emptied into sunat-catalogs and deleted.
const (
	ConsumidorFinalDocNumber = "0"
	ConsumidorFinalName      = "CLIENTES VARIOS"
)
