// Package ring implementa el anillo de GengiSWAP:
// Preparador (plan + QA) -> Chef (entrega) -> Autor/Auditor (decisión).
//
// Principio de diseño: las reglas las hace cumplir el motor, no la buena voluntad
// de ningún modelo. El motor ejecuta él mismo los comandos de verificación y guarda
// la salida literal, comprueba el alcance de los archivos tocados, hace un commit por
// entrega, revierte sin destruir historial (git revert), limita los reintentos y
// persiste el estado en cada paso para poder pausar y retomar.
package ring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrAborted            = errors.New("anillo abortado por el Autor")
	ErrDecisionNotAllowed = errors.New("decisión no permitida en este punto")
	ErrDirtyResume        = errors.New("el chunk quedó interrumpido con cambios sin aprobar: decide continuar o revertir")
	ErrInvalidChunk       = errors.New("chunk inválido")
	ErrPlanNotApproved    = errors.New("plan no aprobado por el Autor")
)

// ---------------------------------------------------------------- decisiones

type Decision int

const (
	GoNext     Decision = iota // aprobar y avanzar
	RetryChunk                 // repetir el mismo chunk con feedback
	Resplit                    // devolver al Preparador para dividir el chunk (REPLAN)
	Abort                      // detener el anillo
)

var decisionNames = map[Decision]string{
	GoNext:     "GO_NEXT",
	RetryChunk: "RETRY_CHUNK",
	Resplit:    "RESPLIT_CHUNK",
	Abort:      "ABORT",
}

func (d Decision) String() string {
	if s, ok := decisionNames[d]; ok {
		return s
	}
	return fmt.Sprintf("DECISION(%d)", int(d))
}

func (d Decision) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Decision) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for k, v := range decisionNames {
		if v == s {
			*d = k
			return nil
		}
	}
	return fmt.Errorf("decisión desconocida %q", s)
}

// Verdict es la resolución del Autor sobre un intento entregado.
type Verdict struct {
	Decision Decision  `json:"decision"`
	Comment  string    `json:"comment"`
	Revert   bool      `json:"revert"` // con RetryChunk: revertir (git revert) el intento antes de repetir
	At       time.Time `json:"at"`
}

// PlanVerdict es la aprobación (o no) del plan completo antes de que el Chef empiece.
type PlanVerdict struct {
	Approved bool
	Comment  string
}

// ---------------------------------------------------------------- alcance

// Scope es la única fuente de lo permitido y prohibido (sección "## Alcance").
type Scope struct {
	CanModify []string `json:"can_modify"`
	ReadOnly  []string `json:"read_only"`
	Forbidden []string `json:"forbidden"`
}

// norm unifica separadores a "/" en cualquier sistema operativo (git y los
// protocolos usan "/", Windows usa "\"), de modo que el alcance se evalúe igual en todos.
func norm(p string) string {
	return strings.TrimSuffix(path.Clean(strings.ReplaceAll(p, `\`, "/")), "/")
}

func under(p string, prefixes []string) bool {
	p = norm(p)
	for _, pre := range prefixes {
		if strings.TrimSpace(pre) == "" {
			continue
		}
		pre = norm(pre)
		if pre == "." || p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}

// Allows indica si el archivo p puede ser modificado según el alcance.
func (s Scope) Allows(p string) bool {
	if n := norm(p); n == ".." || strings.HasPrefix(n, "../") {
		return false
	}
	if under(p, s.Forbidden) || under(p, s.ReadOnly) {
		return false
	}
	return under(p, s.CanModify)
}

// narrows aplica el alcance de un chunk, que solo puede restringir, nunca ampliar.
func (s Scope) narrows(p string) bool {
	if under(p, s.Forbidden) || under(p, s.ReadOnly) {
		return false
	}
	return len(s.CanModify) == 0 || under(p, s.CanModify)
}

func scopeViolations(task, chunk Scope, changed []string) []string {
	var out []string
	for _, f := range changed {
		if !task.Allows(f) || !chunk.narrows(f) {
			out = append(out, f)
		}
	}
	return out
}

func without(all, remove []string) []string {
	drop := map[string]bool{}
	for _, r := range remove {
		drop[norm(r)] = true
	}
	var out []string
	for _, f := range all {
		if !drop[norm(f)] {
			out = append(out, f)
		}
	}
	return out
}

// ---------------------------------------------------------------- modelo de datos

type Status string

const (
	StatusPending   Status = "PENDING"   // [ ] pendiente
	StatusDelivered Status = "DELIVERED" // [~] entregado: espera QA y decisión del Autor
	StatusPartial   Status = "PARTIAL"   // [-] parcial: chunk NO cerrado
	StatusDone      Status = "DONE"      // [x] hecho: QA PASS + GO_NEXT del Autor
	StatusBlocked   Status = "BLOCKED"   // [!] bloqueado
)

type Task struct {
	ID     string
	Prompt string
}

type Attempt struct {
	N               int        `json:"n"`
	StartedAt       time.Time  `json:"started_at"`
	BaseRef         string     `json:"base_ref"`   // HEAD antes de que el Chef empezara
	CommitRef       string     `json:"commit_ref"` // HEAD tras el commit de la entrega
	ChefDone        bool       `json:"chef_done"`
	ChefOutput      string     `json:"chef_output"`
	ChangedFiles    []string   `json:"changed_files"`
	ScopeViolations []string   `json:"scope_violations"`
	Evidence        []Evidence `json:"evidence"` // capturada por el motor, no por un modelo
	PrepPassed      bool       `json:"prep_passed"`
	PrepNotes       string     `json:"prep_notes"`
	QADone          bool       `json:"qa_done"`
	QAPassed        bool       `json:"qa_passed"`
	QANotes         string     `json:"qa_notes"`
	Verdict         *Verdict   `json:"verdict,omitempty"`
}

type Chunk struct {
	ID             string    `json:"id"`
	Spec           string    `json:"spec"`
	Acceptance     []string  `json:"acceptance"`
	VerifyCommands []string  `json:"verify_commands"`
	FailureProof   string    `json:"failure_proof"` // qué cambio haría fallar la verificación
	Scope          Scope     `json:"scope"`         // solo puede restringir el alcance de la tarea
	Status         Status    `json:"status"`
	Attempts       []Attempt `json:"attempts"`
	Feedback       []string  `json:"feedback"`
	ApprovedOutput string    `json:"approved_output"`
}

type Event struct {
	At      time.Time `json:"at"`
	Role    string    `json:"role"` // PREPARADOR | CHEF | AUTOR | MOTOR
	Kind    string    `json:"kind"`
	ChunkID string    `json:"chunk_id,omitempty"`
	Detail  string    `json:"detail"`
}

type State struct {
	TaskID       string  `json:"task_id"`
	Prompt       string  `json:"prompt"`
	Scope        Scope   `json:"scope"`
	MaxRetries   int     `json:"max_retries"`
	PlanApproved bool    `json:"plan_approved"`
	Index        int     `json:"index"`
	Status       string  `json:"status"` // PLAN_PENDING | RUNNING | DONE | ABORTED
	Chunks       []Chunk `json:"chunks"`
	Replaced     []Chunk `json:"replaced,omitempty"` // chunks sustituidos por un REPLAN
	Events       []Event `json:"events"`
}

// LogLines devuelve los eventos con el formato de gengis_global_log.md.
func (s *State) LogLines() []string {
	lines := make([]string, 0, len(s.Events))
	for _, ev := range s.Events {
		ts := ev.At.Format("2006-01-02 15:04:05")
		if ev.Role == "CHEF" {
			lines = append(lines, fmt.Sprintf("[%s] [CHEF] Tarea: %s | Chunk: %s | Estado: %s | Notas: %s",
				ts, s.TaskID, ev.ChunkID, ev.Kind, ev.Detail))
			continue
		}
		chunk := ""
		if ev.ChunkID != "" {
			chunk = "Chunk: " + ev.ChunkID + " | "
		}
		lines = append(lines, fmt.Sprintf("[%s] [%s] %s | Tarea: %s | %s%s", ts, ev.Role, ev.Kind, s.TaskID, chunk, ev.Detail))
	}
	return lines
}

// ---------------------------------------------------------------- actores

type ApprovedChunk struct {
	ID     string
	Output string
}

// ChefInput es todo lo que el Chef puede ver: su chunk, el feedback de intentos
// previos y los chunks ya aprobados (solo lectura).
type ChefInput struct {
	TaskID    string
	Attempt   int
	Chunk     Chunk
	Feedback  []string
	Approved  []ApprovedChunk
	TaskScope Scope
}

type PlanRequest struct {
	TaskID string
	Prompt string
	Scope  Scope
	Chunks []Chunk
}

type ReviewRequest struct {
	TaskID  string
	Chunk   Chunk
	Attempt Attempt
	Allowed []Decision
}

type Preparador interface {
	Split(ctx context.Context, task Task) ([]Chunk, error)
	// Verify es el QA semántico del Preparador (comprobación G5 por muestreo, etc.).
	// Se ejecuta DESPUÉS de que el motor haya corrido los comandos de verificación.
	Verify(ctx context.Context, chunk Chunk, att Attempt) (passed bool, notes string, err error)
	Resplit(ctx context.Context, chunk Chunk) ([]Chunk, error)
}

type Chef interface {
	Execute(ctx context.Context, in ChefInput) (string, error)
}

// Auditor es el Autor (humano): aprueba el plan y decide sobre cada entrega.
type Auditor interface {
	ReviewPlan(ctx context.Context, req PlanRequest) (PlanVerdict, error)
	Review(ctx context.Context, req ReviewRequest) (Verdict, error)
}

// ---------------------------------------------------------------- persistencia

type Store interface {
	Load() (*State, error) // (nil, nil) si no existe estado previo
	Save(*State) error
}

type MemStore struct{ data []byte }

func (m *MemStore) Load() (*State, error) {
	if m.data == nil {
		return nil, nil
	}
	var st State
	if err := json.Unmarshal(m.data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (m *MemStore) Save(st *State) error {
	b, err := json.Marshal(st)
	m.data = b
	return err
}

// FileStore guarda el estado en JSON de forma atómica (archivo temporal + rename).
type FileStore struct{ Path string }

func (f FileStore) Load() (*State, error) {
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("estado corrupto en %s: %w", f.Path, err)
	}
	return &st, nil
}

func (f FileStore) Save(st *State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".ring-state-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), f.Path)
}

// ---------------------------------------------------------------- motor

// ResumePolicy decide qué hacer al retomar un chunk interrumpido que dejó cambios.
type ResumePolicy int

const (
	ResumeAsk      ResumePolicy = iota // por defecto: error, decide el Autor
	ResumeContinue                     // seguir sobre los cambios existentes
	ResumeReset                        // revertir (git revert) y repetir
)

type Engine struct {
	Prep   Preparador
	Chef   Chef
	Aud    Auditor
	Runner CommandRunner
	WS     Workspace
	Store  Store

	QADir         string // si no está vacío, se escribe aquí el informe workspace\qa\<chunk>.md
	OnDirtyResume ResumePolicy
	Now           func() time.Time
}

func validateChunks(chunks []Chunk, existing map[string]bool) error {
	if len(chunks) == 0 {
		return fmt.Errorf("%w: lista vacía", ErrInvalidChunk)
	}
	seen := map[string]bool{}
	for k := range existing {
		seen[k] = true
	}
	for _, c := range chunks {
		switch {
		case strings.TrimSpace(c.ID) == "":
			return fmt.Errorf("%w: sin ID", ErrInvalidChunk)
		case seen[c.ID]:
			return fmt.Errorf("%w: ID duplicado %q", ErrInvalidChunk, c.ID)
		case strings.TrimSpace(c.Spec) == "":
			return fmt.Errorf("%w: %s no tiene spec", ErrInvalidChunk, c.ID)
		case len(c.Acceptance) == 0:
			return fmt.Errorf("%w: %s no tiene criterios de aceptación", ErrInvalidChunk, c.ID)
		case strings.TrimSpace(c.FailureProof) == "":
			return fmt.Errorf("%w: %s no indica la prueba de que la verificación puede fallar", ErrInvalidChunk, c.ID)
		}
		blank := true
		for _, cmd := range c.VerifyCommands {
			if strings.TrimSpace(cmd) != "" {
				blank = false
			}
		}
		if blank {
			return fmt.Errorf("%w: %s no tiene comando de verificación", ErrInvalidChunk, c.ID)
		}
		seen[c.ID] = true
	}
	return nil
}

func checkTerminalState(st *State) (done bool, err error) {
	switch {
	case st.Status == "ABORTED":
		return true, ErrAborted
	case st.Status == "DONE" || st.Index >= len(st.Chunks):
		return true, nil
	default:
		return false, nil
	}
}

func newAttempt(prevCount int, startedAt time.Time, baseRef string) Attempt {
	return Attempt{
		N:         prevCount + 1,
		StartedAt: startedAt,
		BaseRef:   baseRef,
	}
}

func (e *Engine) event(st *State, role, kind, chunkID, detail string) {
	st.Events = append(st.Events, Event{At: e.Now(), Role: role, Kind: kind, ChunkID: chunkID, Detail: detail})
}

func (e *Engine) save(st *State) error {
	if err := e.Store.Save(st); err != nil {
		return fmt.Errorf("guardando estado: %w", err)
	}
	return nil
}

// Run gira el anillo hasta terminar todos los chunks, abortar o fallar. Si el
// Store ya contiene el estado de esta tarea, la retoma donde quedó.
func (e *Engine) Run(ctx context.Context, task Task, scope Scope, maxRetries int) (*State, error) {
	if e.Now == nil {
		e.Now = time.Now
	}
	if e.WS == nil {
		e.WS = NopWorkspace{}
	}
	if maxRetries < 1 {
		maxRetries = 3
	}

	st, err := e.Store.Load()
	if err != nil {
		return nil, fmt.Errorf("cargando estado: %w", err)
	}
	resumed := st != nil
	if st == nil {
		chunks, err := e.Prep.Split(ctx, task)
		if err != nil {
			return nil, fmt.Errorf("dividiendo tarea: %w", err)
		}
		if err := validateChunks(chunks, nil); err != nil {
			return nil, err
		}
		for i := range chunks {
			chunks[i].Status = StatusPending
			chunks[i].Attempts = nil
			chunks[i].Feedback = nil
			chunks[i].ApprovedOutput = ""
		}
		st = &State{TaskID: task.ID, Prompt: task.Prompt, Scope: scope, MaxRetries: maxRetries, Status: "PLAN_PENDING", Chunks: chunks}
		e.event(st, "PREPARADOR", "PLAN_PENDIENTE", "", fmt.Sprintf("%d chunks", len(chunks)))
		if err := e.save(st); err != nil {
			return st, err
		}
	} else {
		if st.TaskID != task.ID {
			return nil, fmt.Errorf("el estado guardado es de la tarea %q, no de %q", st.TaskID, task.ID)
		}
		if done, err := checkTerminalState(st); err != nil {
			return st, err
		} else if done && st.Status == "DONE" {
			return st, nil
		}
	}

	if !st.PlanApproved {
		if err := e.planStep(ctx, st); err != nil {
			return st, err
		}
	}

	for st.Index < len(st.Chunks) {
		i := st.Index
		c := &st.Chunks[i]

		if n := len(c.Attempts); n == 0 || c.Attempts[n-1].Verdict != nil {
			base, err := e.WS.Head(ctx)
			if err != nil {
				return st, fmt.Errorf("leyendo HEAD: %w", err)
			}
			c.Attempts = append(c.Attempts, newAttempt(n, e.Now(), base))
			c.Status = StatusPartial
			if err := e.save(st); err != nil {
				return st, err
			}
		} else if resumed {
			if err := e.checkDirtyResume(ctx, st, i); err != nil {
				return st, err
			}
		}
		resumed = false
		att := &c.Attempts[len(c.Attempts)-1]
		if att.ChefDone {
			c.Status = StatusDelivered
		} else {
			c.Status = StatusPartial
		}

		if !att.ChefDone {
			if err := e.chefStep(ctx, st, i); err != nil {
				return st, err
			}
		}
		if !att.QADone {
			if err := e.qaStep(ctx, st, i); err != nil {
				return st, err
			}
		}
		if err := e.auditStep(ctx, st, i); err != nil {
			return st, err
		}
	}

	st.Status = "DONE"
	e.event(st, "MOTOR", "FIN", "", "todos los chunks aprobados")
	return st, e.save(st)
}

func (e *Engine) planStep(ctx context.Context, st *State) error {
	pv, err := e.Aud.ReviewPlan(ctx, PlanRequest{TaskID: st.TaskID, Prompt: st.Prompt, Scope: st.Scope, Chunks: st.Chunks})
	if err != nil {
		return fmt.Errorf("revisión del plan: %w", err)
	}
	if !pv.Approved {
		st.Status = "PLAN_PENDING"
		e.event(st, "AUTOR", "PLAN_RECHAZADO", "", pv.Comment)
		if serr := e.save(st); serr != nil {
			return serr
		}
		return fmt.Errorf("%w: %s", ErrPlanNotApproved, pv.Comment)
	}
	st.PlanApproved = true
	st.Status = "RUNNING"
	e.event(st, "AUTOR", "PLAN_APROBADO", "", pv.Comment)
	return e.save(st)
}

func (e *Engine) checkDirtyResume(ctx context.Context, st *State, i int) error {
	c := &st.Chunks[i]
	att := &c.Attempts[len(c.Attempts)-1]
	if att.ChefDone {
		return nil // los cambios son del Chef y esperan QA o decisión: es lo esperado
	}
	changed, err := e.WS.ChangedSince(ctx, att.BaseRef)
	if err != nil {
		return fmt.Errorf("comprobando cambios al retomar: %w", err)
	}
	if len(changed) == 0 {
		return nil
	}
	switch e.OnDirtyResume {
	case ResumeContinue:
		e.event(st, "MOTOR", "RETOMA", c.ID, fmt.Sprintf("continúa sobre %d archivos modificados", len(changed)))
		return nil
	case ResumeReset:
		if err := e.WS.Revert(ctx, att.BaseRef); err != nil {
			return fmt.Errorf("revirtiendo al retomar: %w", err)
		}
		e.event(st, "MOTOR", "RETOMA", c.ID, "cambios revertidos (git revert) antes de repetir el chunk")
		return e.save(st)
	default:
		return fmt.Errorf("%w (%s: %s)", ErrDirtyResume, c.ID, strings.Join(changed, ", "))
	}
}

func (e *Engine) chefStep(ctx context.Context, st *State, i int) error {
	c := &st.Chunks[i]
	att := &c.Attempts[len(c.Attempts)-1]

	var approved []ApprovedChunk
	for _, p := range st.Chunks[:i] {
		if p.Status == StatusDone {
			approved = append(approved, ApprovedChunk{ID: p.ID, Output: p.ApprovedOutput})
		}
	}
	view := *c
	view.Attempts = nil

	out, err := e.Chef.Execute(ctx, ChefInput{
		TaskID:    st.TaskID,
		Attempt:   att.N,
		Chunk:     view,
		Feedback:  append([]string(nil), c.Feedback...),
		Approved:  approved,
		TaskScope: st.Scope,
	})
	if err != nil {
		kind := "BLOQUEO"
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			c.Status = StatusPartial // pausa, no fallo
			kind = "PARCIAL"
		} else {
			c.Status = StatusBlocked
		}
		e.event(st, "CHEF", kind, c.ID, err.Error())
		if serr := e.save(st); serr != nil {
			return serr
		}
		return fmt.Errorf("chef en %s: %w", c.ID, err)
	}
	att.ChefOutput = out
	att.ChefDone = true
	c.Status = StatusDelivered
	e.event(st, "CHEF", "ENTREGADO", c.ID, fmt.Sprintf("intento %d entregado", att.N))
	return e.save(st)
}

func (e *Engine) qaStep(ctx context.Context, st *State, i int) error {
	c := &st.Chunks[i]
	att := &c.Attempts[len(c.Attempts)-1]

	changed, err := e.WS.ChangedSince(ctx, att.BaseRef)
	if err != nil {
		return fmt.Errorf("leyendo cambios: %w", err)
	}
	att.ChangedFiles = changed
	att.ScopeViolations = scopeViolations(st.Scope, c.Scope, changed)

	// Un commit por entrega, solo con archivos dentro del alcance. Lo que viole el
	// alcance queda sin commitear y el QA lo marca como FAIL.
	ref, err := e.WS.Commit(ctx, fmt.Sprintf("gengis: %s/%s intento %d", st.TaskID, c.ID, att.N), without(changed, att.ScopeViolations))
	if err != nil {
		return fmt.Errorf("commit de la entrega de %s: %w", c.ID, err)
	}
	att.CommitRef = ref

	att.Evidence = nil
	cmdsOK := true
	for _, cmd := range c.VerifyCommands {
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		ev, err := e.Runner.Run(ctx, cmd)
		if err != nil {
			return fmt.Errorf("ejecutando %q: %w", cmd, err)
		}
		att.Evidence = append(att.Evidence, ev)
		if ev.ExitCode != 0 {
			cmdsOK = false
		}
	}
	e.event(st, "MOTOR", "VERIFICACION", c.ID, fmt.Sprintf("%d comandos, todos ok: %v", len(att.Evidence), cmdsOK))

	semOK, semNotes, err := e.Prep.Verify(ctx, *c, *att)
	if err != nil {
		return fmt.Errorf("QA del Preparador: %w", err)
	}
	att.PrepPassed, att.PrepNotes = semOK, semNotes
	return e.applyQAResult(st, c, att)
}

func qaResultWord(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

func (e *Engine) applyQAResult(st *State, c *Chunk, att *Attempt) error {
	cmdsOK := len(att.Evidence) > 0
	var notes []string
	if len(att.ScopeViolations) > 0 {
		notes = append(notes, "fuera de alcance: "+strings.Join(att.ScopeViolations, ", "))
	}
	for _, ev := range att.Evidence {
		if ev.ExitCode != 0 {
			cmdsOK = false
			notes = append(notes, fmt.Sprintf("`%s` salió con código %d", ev.Command, ev.ExitCode))
		}
	}
	if !att.PrepPassed {
		if strings.TrimSpace(att.PrepNotes) != "" {
			notes = append(notes, "Preparador: "+att.PrepNotes)
		} else {
			notes = append(notes, "Preparador: rechazado en revisión QA")
		}
	}
	att.QANotes = strings.Join(notes, "; ")
	if att.QANotes == "" {
		att.QANotes = att.PrepNotes
	}
	att.QAPassed = len(att.ScopeViolations) == 0 && cmdsOK && att.PrepPassed && len(att.Evidence) > 0
	att.QADone = true

	e.event(st, "PREPARADOR", "QA", c.ID, fmt.Sprintf("Resultado: %s | Notas: %s", qaResultWord(att.QAPassed), att.QANotes))
	if err := e.writeQAReport(st.TaskID, *c); err != nil {
		return err
	}
	return e.save(st)
}

func allowedDecisions(c Chunk, att Attempt, maxRetries int) []Decision {
	var out []Decision
	if att.QAPassed {
		out = append(out, GoNext)
	}
	if len(c.Attempts) < maxRetries {
		out = append(out, RetryChunk)
	}
	return append(out, Resplit, Abort)
}

func contains(ds []Decision, d Decision) bool {
	for _, x := range ds {
		if x == d {
			return true
		}
	}
	return false
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + strings.ToValidUTF8(s[len(s)-n:], "")
}

func feedbackFor(att Attempt, v Verdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Intento %d rechazado (%s).", att.N, v.Decision)
	if att.QANotes != "" {
		fmt.Fprintf(&b, " QA: %s.", att.QANotes)
	}
	if v.Comment != "" {
		fmt.Fprintf(&b, " Autor: %s.", v.Comment)
	}
	for _, ev := range att.Evidence {
		if ev.ExitCode != 0 {
			fmt.Fprintf(&b, "\nSalida de `%s` (código %d):\n%s", ev.Command, ev.ExitCode, tail(ev.Output, 1500))
		}
	}
	return b.String()
}

func (e *Engine) auditStep(ctx context.Context, st *State, i int) error {
	c := &st.Chunks[i]
	att := &c.Attempts[len(c.Attempts)-1]
	allowed := allowedDecisions(*c, *att, st.MaxRetries)

	v, err := e.Aud.Review(ctx, ReviewRequest{TaskID: st.TaskID, Chunk: *c, Attempt: *att, Allowed: allowed})
	if err != nil {
		return fmt.Errorf("revisión del Autor en %s: %w", c.ID, err)
	}
	if !contains(allowed, v.Decision) {
		return fmt.Errorf("%w: %s en %s (permitidas: %v)", ErrDecisionNotAllowed, v.Decision, c.ID, allowed)
	}
	v.At = e.Now()
	att.Verdict = &v
	e.event(st, "AUTOR", "DECISION", c.ID, fmt.Sprintf("Decisión: %s | Motivo: %s", v.Decision, v.Comment))

	switch v.Decision {
	case GoNext:
		c.Status = StatusDone
		c.ApprovedOutput = att.ChefOutput
		st.Index++

	case RetryChunk:
		if v.Revert {
			if err := e.WS.Revert(ctx, att.BaseRef); err != nil {
				att.Verdict = nil
				return fmt.Errorf("revirtiendo %s: %w", c.ID, err)
			}
		} else if len(att.ScopeViolations) > 0 {
			// Fuerza limpieza/stash de los archivos sin commitear fuera de alcance aunque Revert sea false.
			ref := att.CommitRef
			if ref == "" {
				ref = att.BaseRef
			}
			if err := e.WS.Revert(ctx, ref); err != nil {
				att.Verdict = nil
				return fmt.Errorf("limpiando archivos fuera de alcance en %s: %w", c.ID, err)
			}
		}
		c.Feedback = append(c.Feedback, feedbackFor(*att, v))
		c.Status = StatusPartial

	case Resplit:
		if err := e.WS.Revert(ctx, att.BaseRef); err != nil {
			att.Verdict = nil
			return fmt.Errorf("revirtiendo %s: %w", c.ID, err)
		}
		subs, err := e.Prep.Resplit(ctx, *c)
		if err != nil {
			att.Verdict = nil
			return fmt.Errorf("resplit de %s: %w", c.ID, err)
		}
		existing := map[string]bool{c.ID: true}
		for j, o := range st.Chunks {
			if j != i {
				existing[o.ID] = true
			}
		}
		if err := validateChunks(subs, existing); err != nil {
			att.Verdict = nil
			return fmt.Errorf("resplit de %s: %w", c.ID, err)
		}
		for k := range subs {
			subs[k].Status = StatusPending
			subs[k].Attempts = nil
			subs[k].Feedback = nil
		}
		ids := make([]string, len(subs))
		for k, s := range subs {
			ids[k] = s.ID
		}
		old := *c
		st.Replaced = append(st.Replaced, old)
		next := make([]Chunk, 0, len(st.Chunks)-1+len(subs))
		next = append(next, st.Chunks[:i]...)
		next = append(next, subs...)
		next = append(next, st.Chunks[i+1:]...)
		st.Chunks = next
		e.event(st, "PREPARADOR", "REPLAN", "",
			fmt.Sprintf("Chunks afectados: %s → %s | Motivo: %s | Decidido por: autor", old.ID, strings.Join(ids, ","), v.Comment))

	case Abort:
		st.Status = "ABORTED"
		if err := e.save(st); err != nil {
			return err
		}
		return fmt.Errorf("%w en %s: %s", ErrAborted, c.ID, v.Comment)
	}
	return e.save(st)
}

// ---------------------------------------------------------------- informes

func oneLine(s string) string {
	s = strings.NewReplacer("\r\n", " ⏎ ", "\n", " ⏎ ", "\r", " ⏎ ", "`", "'").Replace(s)
	return strings.TrimSpace(s)
}

func firstLine(s string) string {
	if k := strings.IndexByte(s, '\n'); k >= 0 {
		return s[:k]
	}
	return s
}

func safeName(s string) string {
	return strings.NewReplacer("/", "_", `\`, "_", ":", "_").Replace(s)
}

// RenderQAReport genera el contenido de workspace\qa\<chunk>.md con todos los intentos.
func RenderQAReport(taskID string, c Chunk) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# QA del Preparador — %s / %s\n", taskID, c.ID)
	for _, att := range c.Attempts {
		if !att.QADone {
			continue
		}
		fmt.Fprintf(&b, "\n## Intento %d — %s\n", att.N, qaResultWord(att.QAPassed))
		fmt.Fprintf(&b, "- Commit de la entrega: %s (base: %s)\n", orDash(att.CommitRef), orDash(att.BaseRef))
		b.WriteString("\n### (a) Verificación independiente (ejecutada por el motor)\n")
		for _, ev := range att.Evidence {
			fmt.Fprintf(&b, "- comando: `%s` | código de salida: `%d` | últimas líneas: `%s`\n", oneLine(ev.Command), ev.ExitCode, oneLine(tail(ev.Output, 300)))
		}
		b.WriteString("\n### (b) Alcance\n")
		fmt.Fprintf(&b, "- Archivos modificados: %s\n", joinOrNone(att.ChangedFiles))
		fmt.Fprintf(&b, "- Violaciones de alcance: %s\n", joinOrNone(att.ScopeViolations))
		b.WriteString("\n### (c) Revisión del Preparador (G5, muestreo)\n")
		fmt.Fprintf(&b, "- Aprobada: %v\n- Notas: %s\n", att.PrepPassed, orDash(att.PrepNotes))
	}
	return b.String()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func joinOrNone(l []string) string {
	if len(l) == 0 {
		return "ninguno"
	}
	return strings.Join(l, ", ")
}

func (e *Engine) writeQAReport(taskID string, c Chunk) error {
	if e.QADir == "" {
		return nil
	}
	if err := os.MkdirAll(e.QADir, 0o755); err != nil {
		return fmt.Errorf("creando %s: %w", e.QADir, err)
	}
	p := filepath.Join(e.QADir, safeName(c.ID)+".md")
	if err := os.WriteFile(p, []byte(RenderQAReport(taskID, c)), 0o644); err != nil {
		return fmt.Errorf("escribiendo informe QA: %w", err)
	}
	return nil
}

// RenderStatusMD genera el status.md en el formato v2 semántico de GengiSWAP
// (pendiente, entregado, hecho, parcial, bloqueado), con la evidencia G1 bajo cada
// viñeta entregada o hecha y el bloque de detalle para los chunks no cerrados.
func RenderStatusMD(st *State) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Estado de Chunks: %s\n\n", st.TaskID)
	for _, c := range st.Chunks {
		box, word := "[ ]", "pendiente"
		switch c.Status {
		case StatusDelivered:
			box, word = "[~]", "entregado"
		case StatusDone:
			box, word = "[x]", "hecho"
		case StatusPartial:
			box, word = "[-]", "parcial"
		case StatusBlocked:
			box, word = "[!]", "bloqueado"
		}
		fmt.Fprintf(&b, "- %s **%s**: `%s` - %s\n", box, word, c.ID, firstLine(c.Spec))
		if (c.Status == StatusDone || c.Status == StatusDelivered) && len(c.Attempts) > 0 {
			att := c.Attempts[len(c.Attempts)-1]
			for _, ev := range att.Evidence {
				fmt.Fprintf(&b, "  - Evidencia (G1): comando: `%s` | código de salida: `%d` | últimas líneas: `%s`\n",
					oneLine(ev.Command), ev.ExitCode, oneLine(tail(ev.Output, 200)))
			}
			if att.CommitRef != "" {
				fmt.Fprintf(&b, "  - Commit: `%s`\n", att.CommitRef)
			}
		}
	}
	b.WriteString("\n---\n*Convenciones de estado (Protocolo v2):*\n")
	b.WriteString("- `pendiente`: chunk aún no iniciado.\n")
	b.WriteString("- `entregado`: el Chef terminó; espera QA del Preparador y decisión del Autor.\n")
	b.WriteString("- `hecho`: QA PASS y GO_NEXT del Autor, con evidencia literal G1 bajo su viñeta.\n")
	b.WriteString("- `parcial`: chunk NO cerrado; detiene la cadena hasta resolverse.\n")
	b.WriteString("- `bloqueado`: impedimento que requiere intervención del Autor o del Preparador.\n")

	for _, c := range st.Chunks {
		if (c.Status != StatusPartial && c.Status != StatusBlocked) || len(c.Attempts) == 0 {
			continue
		}
		att := c.Attempts[len(c.Attempts)-1]
		tipo, hecho, falta := "interrupción", "ninguno (el Chef no terminó)", "terminar el chunk y entregarlo"
		if att.QADone && !att.QAPassed {
			tipo, hecho, falta = "evidencia incompleta", "Chef entregó y el QA no aprobó", "corregir lo señalado en las notas de QA"
		}
		last := "ninguno"
		if n := len(att.Evidence); n > 0 {
			ev := att.Evidence[n-1]
			last = fmt.Sprintf("`%s` → código %d", oneLine(ev.Command), ev.ExitCode)
		}
		header := "[-] Parcial"
		if c.Status == StatusBlocked {
			header = "[!] Bloqueado"
		}
		fmt.Fprintf(&b, "\n## Chunk %s — %s\n", c.ID, header)
		fmt.Fprintf(&b, "- Tipo: %s\n", tipo)
		fmt.Fprintf(&b, "- Intento: %d\n", att.N)
		fmt.Fprintf(&b, "- Último paso completado: %s\n", hecho)
		fmt.Fprintf(&b, "- Archivos modificados hasta ahora: %s\n", joinOrNone(att.ChangedFiles))
		fmt.Fprintf(&b, "- Último comando ejecutado, código de salida y resumen de su salida: %s\n", last)
		fmt.Fprintf(&b, "- Qué falta: %s\n", falta)
		if att.QANotes != "" {
			fmt.Fprintf(&b, "- Notas de QA: %s\n", att.QANotes)
		}
		b.WriteString("- Para retomar: decide el Autor (continuar | revertir y repetir)\n")
	}
	return b.String()
}
