package specwriter

import "embed"

// TemplatesFS embebe exclusivamente las plantillas Markdown deterministas de GengiSWAP (Opcion 3C).
//
//go:embed templates/*.md.tmpl
var TemplatesFS embed.FS
