package main

// bar91Manuals are JBL's own owner's manuals for the Bar 9.1 True Wireless
// Surround ("JBL SB Bar 9.1 3D SURROUND"), as published on JBL/Harman support
// sites. Each was checked to open the Bar 9.1 manual in that language on
// 5 Nov 2026. They are linked, not bundled: the PDFs are JBL's. Check them
// with JBL_CHECK_MANUALS=1 go test -run TestBar91ManualsOnline .
var bar91Manuals = map[string]string{
	"en": "https://ca.jbl.com/on/demandware.static/-/Sites-masterCatalog_Harman/default/dw8adad6ce/pdfs/JBL_SB_Bar%209.1_3D_SURROUND_Owner%27s%20Manual_EN.pdf",
	"fr": "https://ca.jbl.com/on/demandware.static/-/Sites-masterCatalog_Harman/default/dwdba68517/pdfs/JBL_SB_Bar%209.1_3D_SURROUND_Owner%27s%20Manual_FR.pdf",
	"de": "https://ch.jbl.com/on/demandware.static/-/Sites-masterCatalog_Harman/default/dw1ef036f8/pdfs/JBL_SB_Bar%209.1_3D_SURROUND_Owner%27s%20Manual_DE.pdf",
	"es": "https://support.jbl.com/on/demandware.static/-/Sites-masterCatalog_Harman/default/dw97ddefc9/pdfs/JBL_SB_Bar%209.1_3D_SURROUND_Owner%27s%20Manual_ES.pdf",
	"nl": "https://be.harmanaudio.com/on/demandware.static/-/Sites-masterCatalog_Harman/default/dwfd595418/pdfs/JBL_SB_Bar%209.1_3D_SURROUND_Owner%27s%20Manual_NL.pdf",
}
