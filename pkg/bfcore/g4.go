package bfcore

import (
	"regexp"
	"strings"
)

// G4Tier clasifica cada regla G4 en Tier 1 (secretos duros) o Tier 2 (datos ambientales/PII).
type G4Tier int

const (
	Tier1HardSecret    G4Tier = 1
	Tier2Environmental G4Tier = 2
)

// G4Finding representa una deteccion de la guardia G4 sin transcribir el valor sensible.
type G4Finding struct {
	Rule    string `json:"rule"`
	Tier    G4Tier `json:"tier"`
	Field   string `json:"field,omitempty"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// G4Redaction registra cuantas sustituciones de una regla Tier 2 se aplicaron a un campo.
type G4Redaction struct {
	Rule  string `json:"rule"`
	Field string `json:"field"`
	Count int    `json:"count"`
}

// G4RuleDef define una regla canonica del registro G4 compartido.
type G4RuleDef struct {
	Name    string
	Tier    G4Tier
	Re      *regexp.Regexp
	Message string
}

var (
	reWinUserPath  = regexp.MustCompile(`(?i)\b[A-Z]:[\\/]+Users[\\/]+[^\s\\/"'<>|]+`)
	reUnixUserPath = regexp.MustCompile(`(?:^|[\s"'(=:])(?:/home|/Users)/[A-Za-z0-9._-]+(?:/|$|\s|["')])`)
	// reUnixUserSubgroup captura en grupo 1 el delimitador previo, en grupo 2 el nucleo (?:/home|/Users)/<user> y en grupo 3 el delimitador posterior.
	reUnixUserSubgroup = regexp.MustCompile(`(^|[\s"'(=:])((?:/home|/Users)/[A-Za-z0-9._-]+)(/|$|\s|[:"')\]])`)
	reEmailAddress     = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
)

var g4Rules = []G4RuleDef{
	{
		Name:    "pem_private_key",
		Tier:    Tier1HardSecret,
		Re:      regexp.MustCompile(`(?i)-----BEGIN(?:\s+[A-Z0-9]+)*\s+PRIVATE\s+KEY-----`),
		Message: "bloque de clave privada PEM detectado (G4)",
	},
	{
		Name:    "literal_private_key",
		Tier:    Tier1HardSecret,
		Re:      regexp.MustCompile(`(?i)\bPRIVATE\s+KEY\b`),
		Message: "referencia literal a clave privada detectada (G4)",
	},
	{
		Name:    "win_user_path",
		Tier:    Tier2Environmental,
		Re:      reWinUserPath,
		Message: "ruta absoluta de perfil de usuario Windows detectada (G4)",
	},
	{
		Name:    "unix_user_path",
		Tier:    Tier2Environmental,
		Re:      reUnixUserPath,
		Message: "ruta absoluta de perfil de usuario Unix/macOS detectada (G4)",
	},
	{
		Name:    "bearer_token",
		Tier:    Tier1HardSecret,
		Re:      regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-]{8,}`),
		Message: "token Bearer de autorizacion detectado (G4)",
	},
	{
		Name:    "known_token",
		Tier:    Tier1HardSecret,
		Re:      regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{16,}|github_pat_[A-Za-z0-9_]{20,}|sk-(?:proj-|ant-)?[A-Za-z0-9_\-]{16,}|(?:AKIA|ASIA)[0-9A-Z]{16}|AIza[0-9A-Za-z_\-]{20,}|xox[baprs]-[0-9A-Za-z\-]{10,})\b`),
		Message: "token o API key detectada (G4)",
	},
	{
		Name:    "jwt_token",
		Tier:    Tier1HardSecret,
		Re:      regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{6,}\.eyJ[A-Za-z0-9_\-]{6,}\.[A-Za-z0-9_\-]{6,}\b`),
		Message: "posible token JWT detectado (G4)",
	},
	{
		Name:    "conn_uri_credentials",
		Tier:    Tier1HardSecret,
		Re:      regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql|mariadb|mongodb(?:\+srv)?|rediss?|amqps?|mssql|sqlserver)://[^\s:@/]+:[^\s@/]+@[^\s]+`),
		Message: "cadena de conexion con credenciales detectada (G4)",
	},
	{
		Name:    "conn_password_param",
		Tier:    Tier1HardSecret,
		Re:      regexp.MustCompile(`(?i)(?:^|[;\s"'])(?:password|passwd|pwd)\s*=\s*[^\s;]{3,}`),
		Message: "parametro de contrasena en cadena de conexion detectado (G4)",
	},
	{
		Name:    "secret_assignment",
		Tier:    Tier1HardSecret,
		Re:      regexp.MustCompile(`(?i)\b(?:api_key|apikey|access_token|auth_token|client_secret|secret_key|private_key)\s*[:=]\s*["']?[A-Za-z0-9_\-./+=]{6,}["']?`),
		Message: "asignacion de secreto o clave detectada (G4)",
	},
	{
		Name:    "email_address",
		Tier:    Tier2Environmental,
		Re:      reEmailAddress,
		Message: "correo electronico sensible detectado (G4)",
	},
}

// G4RulesCount devuelve el total de reglas G4 registradas (11).
func G4RulesCount() int {
	return len(g4Rules)
}

func normalizeLineEndings(s string) string {
	s = strings.TrimPrefix(s, "\xef\xbb\xbf")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// ScanG4All ejecuta las 11 reglas G4 en modo fail-closed linea por linea,
// suprimiendo literal_private_key cuando pem_private_key ya coincidio en la misma linea.
func ScanG4All(text string) []G4Finding {
	return ScanTier1("", text, true)
}

// ScanTier1 escanea linea por linea contra las 8 reglas Tier 1 (mas las 3 Tier 2 si strictG4 == true)
// sin almacenar ni transcribir la subcadena sensible encontrada.
func ScanTier1(field, text string, strictG4 bool) []G4Finding {
	lines := strings.Split(normalizeLineEndings(text), "\n")
	var findings []G4Finding

	for idx, line := range lines {
		lineNum := idx + 1
		matchedPEM := false
		for _, rule := range g4Rules {
			if !strictG4 && rule.Tier != Tier1HardSecret {
				continue
			}
			if rule.Name == "literal_private_key" && matchedPEM {
				continue
			}
			if rule.Re.MatchString(line) {
				if rule.Name == "pem_private_key" {
					matchedPEM = true
				}
				findings = append(findings, G4Finding{
					Rule:    rule.Name,
					Tier:    rule.Tier,
					Field:   field,
					Line:    lineNum,
					Message: rule.Message,
				})
			}
		}
	}
	return findings
}

// RedactTier2Text ejecuta un bucle de punto fijo (hasta 16 pasadas hasta que queden 0 coincidencias Tier 2)
// sobre cadenas de texto libre, reemplazando win_user_path y email_address con [REDACTED:G4:<rule>] y
// reemplazando todas las ocurrencias no solapadas del nucleo (?:/home|/Users)/<user> de unix_user_path en cada pasada
// preservando los delimitadores iniciales y finales para que tanto rutas adyacentes (/home/alice /home/bob)
// como decenas de rutas en un mismo texto queden 100% redactadas.
func RedactTier2Text(field, text string) (string, []G4Redaction) {
	if text == "" {
		return "", nil
	}

	cur := text
	winCount := 0
	unixCount := 0
	emailCount := 0

	for pass := 0; pass < 16; pass++ {
		changed := false

		if matches := reWinUserPath.FindAllStringIndex(cur, -1); len(matches) > 0 {
			winCount += len(matches)
			cur = reWinUserPath.ReplaceAllString(cur, "[REDACTED:G4:win_user_path]")
			changed = true
		}

		if subMatches := reUnixUserSubgroup.FindAllStringSubmatchIndex(cur, -1); len(subMatches) > 0 {
			var sb strings.Builder
			sb.Grow(len(cur) + len(subMatches)*16)
			last := 0
			for _, loc := range subMatches {
				sb.WriteString(cur[last:loc[4]])
				sb.WriteString("[REDACTED:G4:unix_user_path]")
				last = loc[5]
			}
			sb.WriteString(cur[last:])
			cur = sb.String()
			unixCount += len(subMatches)
			changed = true
		}

		if matches := reEmailAddress.FindAllStringIndex(cur, -1); len(matches) > 0 {
			emailCount += len(matches)
			cur = reEmailAddress.ReplaceAllString(cur, "[REDACTED:G4:email_address]")
			changed = true
		}

		if !changed {
			break
		}
	}

	var redactions []G4Redaction
	if winCount > 0 {
		redactions = append(redactions, G4Redaction{Rule: "win_user_path", Field: field, Count: winCount})
	}
	if unixCount > 0 {
		redactions = append(redactions, G4Redaction{Rule: "unix_user_path", Field: field, Count: unixCount})
	}
	if emailCount > 0 {
		redactions = append(redactions, G4Redaction{Rule: "email_address", Field: field, Count: emailCount})
	}
	return cur, redactions
}

// RedactAllG4 redacta cualquier coincidencia Tier 1 (primero) y luego Tier 2 a [REDACTED:G4:<rule>]
// para sanitizar metadatos forenses (source, source_ref en REJECTED_G4).
func RedactAllG4(text string) string {
	if text == "" {
		return ""
	}
	cur := text
	for pass := 0; pass < 16; pass++ {
		changed := false
		for _, rule := range g4Rules {
			if rule.Tier != Tier1HardSecret {
				continue
			}
			if rule.Re.MatchString(cur) {
				cur = rule.Re.ReplaceAllString(cur, "[REDACTED:G4:"+rule.Name+"]")
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	cur, _ = RedactTier2Text("", cur)
	return cur
}

// MatchesPersonalPathG4 indica si rawPath coincide con las reglas de rutas personales win_user_path o unix_user_path.
func MatchesPersonalPathG4(rawPath string) bool {
	if rawPath == "" {
		return false
	}
	return reWinUserPath.MatchString(rawPath) || reUnixUserPath.MatchString(rawPath)
}
