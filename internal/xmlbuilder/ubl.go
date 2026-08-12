package xmlbuilder

import sunat "github.com/nmitic/perunio-sunat-catalogs/sunat"

// The prefixed namespaces every SUNAT document declares on its root element.
// Derived, never declared: the URIs live in sunat-catalogs/catalogs/ubl-namespace.json
// and this package must not spell one out again.
var (
	nsCAC = sunat.UblNamespaceUri(sunat.UblNamespaceCAC)
	nsCBC = sunat.UblNamespaceUri(sunat.UblNamespaceCBC)
	nsEXT = sunat.UblNamespaceUri(sunat.UblNamespaceEXT)
	nsDS  = sunat.UblNamespaceUri(sunat.UblNamespaceDS)
	nsSAC = sunat.UblNamespaceUri(sunat.UblNamespaceSAC)
)

// ublRoot is the trio SUNAT checks before anything else: the default namespace of
// the root element, cbc:UBLVersionID and cbc:CustomizationID. Getting one wrong is
// the #1 cause of a silently rejected document — RC and RA are UBL 2.0 while every
// other document is 2.1 — so all three come from one row of ubl-document.json and
// each builder names only which document it is building.
type ublRoot struct {
	NS              string
	UBLVersionID    string
	CustomizationID string
}

func ublRootFor(document string) ublRoot {
	return ublRoot{
		NS:              sunat.UblDocumentNamespace(document),
		UBLVersionID:    sunat.UblDocumentUblVersion(document),
		CustomizationID: sunat.UblDocumentCustomizationId(document),
	}
}
