package model_test

import (
	"testing"

	"maragu.dev/is"

	"github.com/nmitic/perunio-sunat-catalogs/sunat"
	"github.com/perunio/perunio-facturador/internal/model"
)

// The catálogos are moving out of internal/model and into the shared
// perunio-sunat-catalogs module, one catálogo at a time. Until a given catálogo
// has moved, this pins the two definitions together: the migration is then a
// rename, and any disagreement fails here rather than at SUNAT.
//
// Delete each subtest as its catálogo's constants leave model/catalog.go.
func TestSharedCatalogAgreesWithModel(t *testing.T) {
	t.Run("should give Cat.16 the same codes model already names", func(t *testing.T) {
		is.Equal(t, model.PriceTypeUnitWithIGV, sunat.Cat16PrecioUnitario)
		is.Equal(t, model.PriceTypeReferential, sunat.Cat16ValorReferencial)
	})

	t.Run("should know the Cat.16 code the XML builder accepts but model never named", func(t *testing.T) {
		// xmlbuilder.validPriceTypeCodes accepts 01, 02 and 03, while
		// model/catalog.go names only the first two. The shared catálogo carries
		// all three, so the constant exists before the builder needs it.
		is.True(t, sunat.Cat16Valid(sunat.Cat16ValorReferencialExportacion))
		is.Equal(t, "03", sunat.Cat16ValorReferencialExportacion)
	})

	t.Run("should reject a code SUNAT does not define", func(t *testing.T) {
		is.True(t, !sunat.Cat16Valid("99"))

		_, ok := sunat.Cat16("99")
		is.True(t, !ok)
	})

	t.Run("should keep every selectable code inside the catálogo", func(t *testing.T) {
		// A picker must never offer a code the pipeline would reject. This is the
		// invariant the scattered per-repo copies used to violate.
		for _, code := range sunat.Cat16SelectCodes {
			is.True(t, sunat.Cat16Valid(code))
			is.True(t, sunat.Cat16Emit(code))
		}
	})
}
