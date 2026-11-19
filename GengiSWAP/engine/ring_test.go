package ring

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- actores simulados

type fakeWS struct {
	head    string
	changed []string
	commits []string // mensajes
	files   [][]string
	reverts int
	bases   []string
}

func (w *fakeWS) Head(context.Context) (string, error) { return w.head, nil }
func (w *fakeWS) ChangedSince(context.Context, string) ([]string, error) {
	return append([]string(nil), w.changed...), nil
}
func (w *fakeWS) Commit(_ context.Context, msg string, files []string) (string, error) {
	w.commits = append(w.commits, msg)
	w.files = append(w.files, append([]string(nil), files...))
	return fmt.Sprintf("c%d", len(w.commits)), nil
}
func (w *fakeWS) Revert(_ context.Context, base string) error {
	w.reverts++
	w.bases = append(w.bases, base)
	return nil
}

type fakeRunner struct {
	fn    func(cmd string, call int) Evidence
	calls map[string]int
}

func (r *fakeRunner) Run(_ context.Context, command string) (Evidence, error) {
	if r.calls == nil {
		r.calls = map[string]int{}
	}
	r.calls[command]++
	ev := r.fn(command, r.calls[command])
	ev.Command = command
	return ev, nil
}

func okRunner() *fakeRunner {
	return &fakeRunner{fn: func(string, int) Evidence { return Evidence{Output: "PASS"} }}
}

type fakePrep struct {
	chunks []Chunk
	subs   []Chunk
	splits int
}

func (p *fakePrep) Split(context.Context, Task) ([]Chunk, error) {
	p.splits++
	return append([]Chunk(nil), p.chunks...), nil
}
func (p *fakePrep) Verify(context.Context, Chunk, Attempt) (bool, string, error) {
	return true, "ok", nil
}
func (p *fakePrep) Resplit(context.Context, Chunk) ([]Chunk, error) {
	return append([]Chunk(nil), p.subs...), nil
}

type fakeChef struct {
	inputs []ChefInput
	errAt  map[int]error
}

func (c *fakeChef) Execute(_ context.Context, in ChefInput) (string, error) {
	c.inputs = append(c.inputs, in)
	if err, ok := c.errAt[len(c.inputs)]; ok {
		return "", err
	}
	return fmt.Sprintf("out-%s-%d", in.Chunk.ID, in.Attempt), nil
}

type fnChef struct {
	fn func(ctx context.Context, in ChefInput) (string, error)
}

func (f fnChef) Execute(ctx context.Context, in ChefInput) (string, error) {
	return f.fn(ctx, in)
}

type fakeAud struct {
	plan    PlanVerdict
	plans   int
	script  []Verdict
	reqs    []ReviewRequest
	crashOn int
}

func (a *fakeAud) ReviewPlan(context.Context, PlanRequest) (PlanVerdict, error) {
	a.plans++
	return a.plan, nil
}

func (a *fakeAud) Review(_ context.Context, req ReviewRequest) (Verdict, error) {
	a.reqs = append(a.reqs, req)
	n := len(a.reqs)
	if a.crashOn == n {
		return Verdict{}, errors.New("crash simulado")
	}
	if n > len(a.script) {
		return Verdict{}, errors.New("sin veredicto en el guion")
	}
	return a.script[n-1], nil
}

func mk(ids ...string) []Chunk {
	var out []Chunk
	for _, id := range ids {
		out = append(out, Chunk{
			ID:             id,
			Spec:           "spec " + id,
			Acceptance:     []string{"criterio " + id},
			VerifyCommands: []string{"verify " + id},
			FailureProof:   "borrar la función de " + id,
		})
	}
	return out
}

func v(d Decision) Verdict { return Verdict{Decision: d, Comment: "c-" + d.String()} }

type rig struct {
	prep  *fakePrep
	chef  *fakeChef
	aud   *fakeAud
	run   *fakeRunner
	ws    *fakeWS
	store Store
	eng   *Engine
}

func newRig(chunks []Chunk, script ...Verdict) *rig {
	r := &rig{
		prep:  &fakePrep{chunks: chunks},
		chef:  &fakeChef{},
		aud:   &fakeAud{plan: PlanVerdict{Approved: true}, script: script},
		run:   okRunner(),
		ws:    &fakeWS{head: "h0"},
		store: &MemStore{},
	}
	r.eng = &Engine{}
	return r
}

func (r *rig) exec(maxRetries int) (*State, error) {
	r.eng.Prep, r.eng.Chef, r.eng.Aud, r.eng.Runner, r.eng.WS, r.eng.Store = r.prep, r.chef, r.aud, r.run, r.ws, r.store
	return r.eng.Run(context.Background(), Task{ID: "T1", Prompt: "p"}, Scope{CanModify: []string{"src"}}, maxRetries)
}

func logHas(st *State, parts ...string) bool {
	for _, l := range st.LogLines() {
		ok := true
		for _, p := range parts {
			if !strings.Contains(l, p) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- plan

func TestPlanMustBeApprovedBeforeChefStarts(t *testing.T) {
	r := newRig(mk("A"), v(GoNext))
	r.aud.plan = PlanVerdict{Approved: false, Comment: "criterios flojos"}
	st, err := r.exec(3)
	if !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("esperaba ErrPlanNotApproved, got %v", err)
	}
	if len(r.chef.inputs) != 0 {
		t.Fatal("el Chef no puede empezar sin PLAN_APROBADO")
	}
	if st.Status != "PLAN_PENDING" || !logHas(st, "[PREPARADOR] PLAN_PENDIENTE") || !logHas(st, "[AUTOR] PLAN_RECHAZADO") {
		t.Fatalf("estado/log incorrectos: %s\n%s", st.Status, strings.Join(st.LogLines(), "\n"))
	}

	// el plan pendiente se retoma sin volver a dividir la tarea
	r.aud.plan = PlanVerdict{Approved: true}
	st, err = r.exec(3)
	if err != nil || st.Status != "DONE" {
		t.Fatalf("err=%v", err)
	}
	if r.prep.splits != 1 {
		t.Fatalf("el plan no debía dividirse de nuevo (splits=%d)", r.prep.splits)
	}
	if !logHas(st, "[AUTOR] PLAN_APROBADO") {
		t.Fatal("falta PLAN_APROBADO en el log")
	}
}

func TestPlanApprovalNotAskedTwiceOnResume(t *testing.T) {
	store := &MemStore{}
	r1 := newRig(mk("A"), v(GoNext))
	r1.store = store
	r1.aud.crashOn = 1
	_, _ = r1.exec(3)

	r2 := newRig(mk("A"), v(GoNext))
	r2.store = store
	if _, err := r2.exec(3); err != nil {
		t.Fatal(err)
	}
	if r2.aud.plans != 0 {
		t.Fatal("un plan ya aprobado no debe volver a pedirse al reanudar")
	}
}

// ---------------------------------------------------------------- flujo

func TestHappyPathDeliveryCommitAndStates(t *testing.T) {
	r := newRig(mk("A", "B"), v(GoNext), v(GoNext))
	r.ws.changed = []string{"src/a.go"}
	st, err := r.exec(3)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "DONE" || st.Chunks[0].Status != StatusDone || st.Chunks[1].Status != StatusDone {
		t.Fatalf("estado inesperado: %s", st.Status)
	}
	// UN commit por entrega, con el mensaje del protocolo y solo los archivos permitidos
	if len(r.ws.commits) != 2 || r.ws.commits[0] != "gengis: T1/A intento 1" || r.ws.commits[1] != "gengis: T1/B intento 1" {
		t.Fatalf("commits: %v", r.ws.commits)
	}
	if len(r.ws.files[0]) != 1 || r.ws.files[0][0] != "src/a.go" {
		t.Fatalf("archivos commiteados: %v", r.ws.files)
	}
	if st.Chunks[0].Attempts[0].CommitRef != "c1" || st.Chunks[0].Attempts[0].BaseRef != "h0" {
		t.Fatalf("referencias git: %+v", st.Chunks[0].Attempts[0])
	}
	// el Chef del chunk B ve el resultado aprobado de A (solo lectura)
	ap := r.chef.inputs[1].Approved
	if len(ap) != 1 || ap[0].ID != "A" || ap[0].Output != "out-A-1" {
		t.Fatalf("contexto acumulado incorrecto: %+v", ap)
	}
	// la verificación la ejecutó el motor, una vez por chunk
	if r.run.calls["verify A"] != 1 || r.run.calls["verify B"] != 1 {
		t.Fatalf("verificaciones: %v", r.run.calls)
	}
	// el Autor siempre ve el chunk como entregado y con QA PASS
	if !r.aud.reqs[0].Attempt.QAPassed || r.aud.reqs[0].Chunk.Status != StatusDelivered {
		t.Fatalf("el Autor debe revisar un chunk entregado con QA PASS: %+v", r.aud.reqs[0].Chunk.Status)
	}
	for _, want := range [][]string{
		{"[CHEF]", "Chunk: A", "Estado: ENTREGADO"},
		{"[PREPARADOR]", "QA", "Chunk: A", "Resultado: PASS"},
		{"[AUTOR]", "DECISION", "Chunk: A", "Decisión: GO_NEXT"},
	} {
		if !logHas(st, want...) {
			t.Errorf("falta en el log %v:\n%s", want, strings.Join(st.LogLines(), "\n"))
		}
	}
}

func TestChefCannotCloseOwnChunk(t *testing.T) {
	// aunque todo pase, el chunk queda "entregado" hasta que el Autor decida
	r := newRig(mk("A"), v(GoNext))
	r.aud.crashOn = 1
	st, err := r.exec(3)
	if err == nil {
		t.Fatal("esperaba el crash simulado antes de la decisión")
	}
	if got := st.Chunks[0].Status; got != StatusDelivered {
		t.Fatalf("sin decisión del Autor el chunk debe estar entregado, no %s", got)
	}
	if !strings.Contains(RenderStatusMD(st), "- [~] **entregado**: `A`") {
		t.Fatal("status.md debe mostrar [~] entregado")
	}
}

func TestRetryCarriesEvidenceIntoFeedback(t *testing.T) {
	r := newRig(mk("A"), v(RetryChunk), v(GoNext))
	r.run.fn = func(cmd string, call int) Evidence {
		if call == 1 {
			return Evidence{ExitCode: 1, Output: "boom: tabla no existe"}
		}
		return Evidence{Output: "PASS"}
	}
	st, err := r.exec(3)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(st.Chunks[0].Attempts); got != 2 {
		t.Fatalf("esperaba 2 intentos, hubo %d", got)
	}
	fb := r.chef.inputs[1].Feedback
	if len(fb) != 1 || !strings.Contains(fb[0], "boom: tabla no existe") {
		t.Fatalf("el feedback debe llevar la salida literal del comando: %v", fb)
	}
	if contains(r.aud.reqs[0].Allowed, GoNext) {
		t.Fatal("con QA fallido no debe permitirse GO_NEXT")
	}
	if r.ws.commits[1] != "gengis: T1/A intento 2" {
		t.Fatalf("el segundo intento debe tener su propio commit: %v", r.ws.commits)
	}
}

func TestRetryWithRevertUsesAttemptBase(t *testing.T) {
	r := newRig(mk("A"), Verdict{Decision: RetryChunk, Revert: true}, v(GoNext))
	r.run.fn = func(cmd string, call int) Evidence {
		if call == 1 {
			return Evidence{ExitCode: 1}
		}
		return Evidence{Output: "PASS"}
	}
	if _, err := r.exec(3); err != nil {
		t.Fatal(err)
	}
	if r.ws.reverts != 1 || r.ws.bases[0] != "h0" {
		t.Fatalf("la reversión debe apuntar a la base del intento: reverts=%d bases=%v", r.ws.reverts, r.ws.bases)
	}
}

func TestRetryScopeViolationForcesRevertEvenWhenRevertFalse(t *testing.T) {
	r := newRig(mk("A"), Verdict{Decision: RetryChunk, Revert: false, Comment: "quita lo prohibido"}, v(GoNext))
	r.ws.changed = []string{"src/ok.go", "docs/prohibido.md"}
	call := 0
	r.run = &fakeRunner{fn: func(string, int) Evidence {
		call++
		if call == 1 {
			r.ws.changed = []string{"src/ok.go"}
		}
		return Evidence{Output: "PASS"}
	}}
	st, err := r.exec(3)
	if err != nil {
		t.Fatalf("err=%v st=%+v", err, st)
	}
	if r.ws.reverts != 1 {
		t.Fatalf("esperaba 1 revert por violación de alcance aunque Verdict.Revert=false, hubo %d", r.ws.reverts)
	}
	if len(r.ws.bases) != 1 || r.ws.bases[0] != "c1" {
		t.Fatalf("debía limpiar desde CommitRef c1 para preservar el commit permitido, bases=%v", r.ws.bases)
	}
}

func TestCannotApproveFailingQA(t *testing.T) {
	r := newRig(mk("A"), v(GoNext))
	r.run.fn = func(string, int) Evidence { return Evidence{ExitCode: 2, Output: "fallo"} }
	st, err := r.exec(3)
	if !errors.Is(err, ErrDecisionNotAllowed) {
		t.Fatalf("esperaba ErrDecisionNotAllowed, got %v", err)
	}
	if st.Chunks[0].Status == StatusDone {
		t.Fatal("un chunk con QA fallido no puede cerrarse")
	}
}

func TestRetryLimitForcesDecision(t *testing.T) {
	failA := func(cmd string, _ int) Evidence {
		if cmd == "verify A" {
			return Evidence{ExitCode: 1, Output: "sigue fallando"}
		}
		return Evidence{Output: "PASS"}
	}

	t.Run("reintentar más allá del límite se rechaza", func(t *testing.T) {
		r := newRig(mk("A"), v(RetryChunk), v(RetryChunk))
		r.run.fn = failA
		_, err := r.exec(2)
		if !errors.Is(err, ErrDecisionNotAllowed) {
			t.Fatalf("esperaba ErrDecisionNotAllowed, got %v", err)
		}
		if contains(r.aud.reqs[1].Allowed, RetryChunk) {
			t.Fatal("en el último intento no debe ofrecerse RETRY_CHUNK")
		}
	})

	t.Run("resplit sustituye el chunk y registra REPLAN", func(t *testing.T) {
		r := newRig(mk("A"), v(RetryChunk), v(Resplit), v(GoNext), v(GoNext))
		r.prep.subs = mk("A1", "A2")
		r.run.fn = failA
		st, err := r.exec(2)
		if err != nil {
			t.Fatal(err)
		}
		if len(st.Chunks) != 2 || st.Chunks[0].ID != "A1" || st.Status != "DONE" {
			t.Fatalf("chunks tras resplit: %+v", st.Chunks)
		}
		if len(st.Replaced) != 1 || st.Replaced[0].ID != "A" {
			t.Fatal("el chunk original debe archivarse en Replaced")
		}
		if r.ws.reverts != 1 {
			t.Fatalf("resplit debe revertir el intento sin destruir historial (reverts=%d)", r.ws.reverts)
		}
		if !logHas(st, "REPLAN | Tarea: T1 | Chunks afectados: A → A1,A2", "Decidido por: autor") {
			t.Fatalf("falta la línea REPLAN en el log:\n%s", strings.Join(st.LogLines(), "\n"))
		}
	})
}

func TestResplitRejectsSubchunksWithoutFailureProof(t *testing.T) {
	r := newRig(mk("A"), v(Resplit))
	r.prep.subs = []Chunk{{ID: "A1", Spec: "s", Acceptance: []string{"x"}, VerifyCommands: []string{"v"}}}
	_, err := r.exec(3)
	if !errors.Is(err, ErrInvalidChunk) {
		t.Fatalf("esperaba ErrInvalidChunk, got %v", err)
	}
}

func TestScopeViolationFailsQAAndIsNotCommitted(t *testing.T) {
	r := newRig(mk("A"), v(Abort))
	r.ws.changed = []string{"src/ok.go", "docs/prohibido.md"}
	_, err := r.exec(3)
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("esperaba ErrAborted, got %v", err)
	}
	att := r.aud.reqs[0].Attempt
	if att.QAPassed || len(att.ScopeViolations) != 1 || att.ScopeViolations[0] != "docs/prohibido.md" {
		t.Fatalf("violación de alcance no detectada: %+v", att)
	}
	if contains(r.aud.reqs[0].Allowed, GoNext) {
		t.Fatal("con violación de alcance no debe permitirse GO_NEXT aunque los comandos pasen")
	}
	if len(r.ws.files[0]) != 1 || r.ws.files[0][0] != "src/ok.go" {
		t.Fatalf("lo que viola el alcance no debe commitearse: %v", r.ws.files)
	}
}

func TestWithoutNormalizesSeparators(t *testing.T) {
	all := []string{`src\ok.go`, `docs\prohibido.md`, "a/b/c.go", "./extra/x.go"}
	remove := []string{"docs/prohibido.md", `a\b\c.go`, "extra/x.go"}
	got := without(all, remove)
	if len(got) != 1 || got[0] != `src\ok.go` {
		t.Fatalf("without(%v, %v) = %v, esperaba [src\\ok.go]", all, remove, got)
	}
}

func TestChunkScopeCanOnlyNarrow(t *testing.T) {
	task := Scope{CanModify: []string{"src"}}
	chunk := Scope{CanModify: []string{"src/api"}}
	got := scopeViolations(task, chunk, []string{"src/api/x.go", "src/db/y.go", "otro/z.go"})
	if len(got) != 2 || got[0] != "src/db/y.go" || got[1] != "otro/z.go" {
		t.Fatalf("violaciones: %v", got)
	}
	wide := Scope{CanModify: []string{"."}}
	if got := scopeViolations(task, wide, []string{"otro/z.go"}); len(got) != 1 {
		t.Fatal("el alcance del chunk no debe poder ampliar el de la tarea")
	}
}

func TestAbortPersists(t *testing.T) {
	r := newRig(mk("A"), v(Abort))
	st, err := r.exec(3)
	if !errors.Is(err, ErrAborted) || st.Status != "ABORTED" {
		t.Fatalf("err=%v status=%s", err, st.Status)
	}
	if _, err := r.exec(3); !errors.Is(err, ErrAborted) {
		t.Fatalf("un anillo abortado no debe reanudarse solo: %v", err)
	}
}

func TestResumeAfterCrashDoesNotRepeatWork(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "state.json")}

	r1 := newRig(mk("A"), v(GoNext))
	r1.store = store
	r1.aud.crashOn = 1
	if _, err := r1.exec(3); err == nil {
		t.Fatal("esperaba el crash simulado")
	}

	r2 := newRig(mk("A"), v(GoNext))
	r2.store = store
	st, err := r2.exec(3)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "DONE" {
		t.Fatalf("status=%s", st.Status)
	}
	if r2.prep.splits != 0 || len(r2.chef.inputs) != 0 || len(r2.run.calls) != 0 || len(r2.ws.commits) != 0 {
		t.Fatalf("al reanudar no debe repetirse split/chef/verificación/commit: splits=%d chef=%d runner=%v commits=%v",
			r2.prep.splits, len(r2.chef.inputs), r2.run.calls, r2.ws.commits)
	}
}

func TestDirtyResumePolicies(t *testing.T) {
	store := &MemStore{}

	r1 := newRig(mk("A"))
	r1.store = store
	r1.ws.changed = []string{"src/a.go"}
	r1.chef.errAt = map[int]error{1: context.Canceled} // pausa a mitad del Chef
	st, err := r1.exec(3)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if st.Chunks[0].Status != StatusPartial || !logHas(st, "[CHEF]", "Estado: PARCIAL") {
		t.Fatalf("una pausa debe dejar el chunk en parcial y registrarlo como PARCIAL, no %s", st.Chunks[0].Status)
	}

	r2 := newRig(mk("A"), v(GoNext))
	r2.store = store
	r2.ws.changed = []string{"src/a.go"}
	if _, err := r2.exec(3); !errors.Is(err, ErrDirtyResume) {
		t.Fatalf("por defecto debe pedir decisión al Autor: %v", err)
	}
	if len(r2.chef.inputs) != 0 {
		t.Fatal("no debe ejecutar al Chef mientras el Autor no decide")
	}

	r3 := newRig(mk("A"), v(GoNext))
	r3.store = store
	r3.ws.changed = []string{"src/a.go"}
	r3.eng.OnDirtyResume = ResumeReset
	st, err = r3.exec(3)
	if err != nil || st.Status != "DONE" {
		t.Fatalf("err=%v", err)
	}
	if r3.ws.reverts != 1 {
		t.Fatalf("ResumeReset debía revertir una vez, reverts=%d", r3.ws.reverts)
	}
}

func TestChefErrorBlocksChunk(t *testing.T) {
	r := newRig(mk("A"))
	r.chef.errAt = map[int]error{1: errors.New("símbolo inexistente (G5)")}
	st, err := r.exec(3)
	if err == nil {
		t.Fatal("esperaba error del Chef")
	}
	if st.Chunks[0].Status != StatusBlocked || !logHas(st, "[CHEF]", "Estado: BLOQUEO") {
		t.Fatalf("estado=%s", st.Chunks[0].Status)
	}
	md := RenderStatusMD(st)
	if !strings.Contains(md, "- [!] **bloqueado**: `A`") {
		t.Fatal("status.md debe mostrar [!] bloqueado")
	}
	if !strings.Contains(md, "## Chunk A — [!] Bloqueado") {
		t.Fatalf("el encabezado inferior debe mostrar [!] Bloqueado:\n%s", md)
	}
}

func TestRejectsInvalidChunks(t *testing.T) {
	noProof := mk("A")
	noProof[0].FailureProof = ""
	cases := map[string][]Chunk{
		"sin verificación":               {{ID: "A", Spec: "s", Acceptance: []string{"x"}, FailureProof: "p"}},
		"sin criterios":                  {{ID: "A", Spec: "s", VerifyCommands: []string{"v"}, FailureProof: "p"}},
		"sin prueba de que puede fallar": noProof,
		"id duplicado":                   append(mk("A"), mk("A")...),
		"lista vacía":                    nil,
	}
	for name, chunks := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(chunks)
			if _, err := r.exec(3); !errors.Is(err, ErrInvalidChunk) {
				t.Fatalf("esperaba ErrInvalidChunk, got %v", err)
			}
			if st, _ := r.store.Load(); st != nil {
				t.Fatal("un plan inválido no debe persistirse")
			}
		})
	}
}

// ---------------------------------------------------------------- informes

func TestRenderStatusMDFormat(t *testing.T) {
	r := newRig(mk("A", "B"), v(GoNext))
	r.aud.crashOn = 2
	r.run.fn = func(cmd string, _ int) Evidence {
		if cmd == "verify B" {
			return Evidence{ExitCode: 1, Output: "rojo"}
		}
		return Evidence{Output: "verde"}
	}
	st, err := r.exec(3)
	if err == nil {
		t.Fatal("esperaba el crash simulado en la segunda revisión")
	}
	// B quedó entregado con QA FAIL; para ver el bloque parcial lo marcamos como el motor lo haría tras un RETRY
	st.Chunks[1].Status = StatusPartial
	md := RenderStatusMD(st)
	for _, want := range []string{
		"# Estado de Chunks: T1",
		"- [x] **hecho**: `A`",
		"  - Evidencia (G1): comando: `verify A` | código de salida: `0` | últimas líneas: `verde`",
		"  - Commit: `c1`",
		"- [-] **parcial**: `B`",
		"## Chunk B — [-] Parcial",
		"Tipo: evidencia incompleta",
		"Último comando ejecutado, código de salida y resumen de su salida: `verify B` → código 1",
		"Para retomar:",
		"`entregado`: el Chef terminó",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("falta %q en:\n%s", want, md)
		}
	}
}

func TestRenderStatusMDBlockedHeader(t *testing.T) {
	st := &State{
		TaskID: "T1",
		Chunks: []Chunk{
			{
				ID:       "A",
				Spec:     "chunk bloqueado",
				Status:   StatusBlocked,
				Attempts: []Attempt{{N: 1, ChangedFiles: []string{"src/a.go"}}},
			},
			{
				ID:       "B",
				Spec:     "chunk parcial",
				Status:   StatusPartial,
				Attempts: []Attempt{{N: 1, QADone: true, QAPassed: false, QANotes: "falló verify"}},
			},
		},
	}
	md := RenderStatusMD(st)
	for _, want := range []string{
		"- [!] **bloqueado**: `A`",
		"- [-] **parcial**: `B`",
		"## Chunk A — [!] Bloqueado",
		"## Chunk B — [-] Parcial",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("falta %q en:\n%s", want, md)
		}
	}
}

func TestQAReportWritten(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "workspace", "qa")
	r := newRig(mk("A"), v(GoNext))
	r.eng.QADir = dir
	r.ws.changed = []string{"src/a.go", "x/fuera.go"}
	if _, err := r.exec(3); err == nil {
		// la violación de alcance impide GO_NEXT: el error es esperado
		t.Fatal("esperaba rechazo de GO_NEXT por violación de alcance")
	}
	b, err := os.ReadFile(filepath.Join(dir, "A.md"))
	if err != nil {
		t.Fatalf("no se escribió el informe QA: %v", err)
	}
	s := string(b)
	for _, want := range []string{"# QA del Preparador — T1 / A", "## Intento 1 — FAIL", "(a) Verificación independiente", "(b) Alcance", "x/fuera.go", "(c) Revisión del Preparador (G5, muestreo)"} {
		if !strings.Contains(s, want) {
			t.Errorf("falta %q en:\n%s", want, s)
		}
	}
}

func TestDecisionJSONRoundTrip(t *testing.T) {
	store := &MemStore{}
	r := newRig(mk("A"), v(RetryChunk))
	r.store = store
	r.run.fn = func(string, int) Evidence { return Evidence{ExitCode: 1} }
	_, _ = r.exec(3) // termina con error de guion agotado; el estado ya está guardado
	st, err := store.Load()
	if err != nil || st == nil {
		t.Fatalf("no se pudo recargar: %v", err)
	}
	if got := st.Chunks[0].Attempts[0].Verdict.Decision; got != RetryChunk {
		t.Fatalf("la decisión no sobrevivió a JSON: %v", got)
	}
	if !st.PlanApproved {
		t.Fatal("la aprobación del plan debe persistir")
	}
}

func TestScopeAllows(t *testing.T) {
	s := Scope{CanModify: []string{"src", "tests/"}, ReadOnly: []string{"src/vendor", "workspace/verificacion"}, Forbidden: []string{"src/secrets.env"}}
	cases := map[string]bool{
		"src/a.go":                    true,
		"src":                         true,
		"tests/x_test.go":             true,
		"src/vendor/lib.go":           false,
		"src/secrets.env":             false,
		"srcfake/a.go":                false, // prefijo de nombre, no de directorio
		"docs/readme.md":              false,
		"../fuera.go":                 false,
		"src/../docs/hack.md":         false,
		`src\windows\path.go`:         true,
		`src\vendor\lib.go`:           false,
		"workspace/verificacion/t.sh": false,
		"":                            false,
	}
	for p, want := range cases {
		if got := s.Allows(p); got != want {
			t.Errorf("Allows(%q) = %v, esperaba %v", p, got, want)
		}
	}
}

// ---------------------------------------------------------------- runner y git

func TestShellRunner(t *testing.T) {
	r := ShellRunner{MaxOutput: 40}
	ev, err := r.Run(context.Background(), "exit 3")
	if err != nil || ev.ExitCode != 3 {
		t.Fatalf("exit 3 -> ev=%+v err=%v", ev, err)
	}
	ev, err = r.Run(context.Background(), "echo hola")
	if err != nil || ev.ExitCode != 0 || !strings.Contains(ev.Output, "hola") {
		t.Fatalf("echo -> ev=%+v err=%v", ev, err)
	}
	ev, _ = r.Run(context.Background(), "echo 0123456789012345678901234567890123456789012345678901234567890123456789")
	if !ev.Truncated {
		t.Fatal("la salida larga debía truncarse")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Run(ctx, "echo x"); err == nil {
		t.Fatal("un contexto cancelado debe devolver error, no evidencia")
	}
}

func TestShellRunnerCmdLineSpacesExitAndOutput(t *testing.T) {
	if os.PathSeparator != '\\' {
		t.Skip("solo aplica a Windows")
	}
	r := ShellRunner{}
	dir := filepath.Join(t.TempDir(), "Proyecto Tungsteno", "sub dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	argFile := filepath.Join(dir, "archivo con espacios.txt")
	if err := os.WriteFile(argFile, []byte("CONTENIDO_OK\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptOK := filepath.Join(dir, "script prueba.cmd")
	if err := os.WriteFile(scriptOK, []byte("@echo off\r\ntype \"%~1\"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptFail := filepath.Join(dir, "script falla.cmd")
	if err := os.WriteFile(scriptFail, []byte("@echo off\r\necho STDOUT_CAPTURADO\r\necho STDERR_CAPTURADO 1>&2\r\nexit /b 7\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Caso 1a: comando sin comillas + ruta con espacios entre comillas.
	ev, err := r.Run(context.Background(), `type "`+argFile+`"`)
	if err != nil || ev.ExitCode != 0 || !strings.Contains(ev.Output, "CONTENIDO_OK") {
		t.Fatalf("type ruta con espacios -> ev=%+v err=%v", ev, err)
	}

	// Caso 1b: ejecutable/script y argumento, ambos con espacios y comillas.
	ev, err = r.Run(context.Background(), `"`+scriptOK+`" "`+argFile+`"`)
	if err != nil || ev.ExitCode != 0 || !strings.Contains(ev.Output, "CONTENIDO_OK") {
		t.Fatalf("script+arg con espacios -> ev=%+v err=%v", ev, err)
	}

	// Casos 2 y 3: exit distinto de 0 y salida capturada (stdout + stderr).
	ev, err = r.Run(context.Background(), `"`+scriptFail+`"`)
	if err != nil {
		t.Fatalf("error inesperado ejecutando script con exit 7: %v", err)
	}
	if ev.ExitCode != 7 {
		t.Fatalf("ExitCode esperado 7, obtenido %d (output=%q)", ev.ExitCode, ev.Output)
	}
	if !strings.Contains(ev.Output, "STDOUT_CAPTURADO") || !strings.Contains(ev.Output, "STDERR_CAPTURADO") {
		t.Fatalf("se esperaba stdout y stderr capturados, output=%q", ev.Output)
	}
}

func TestShellRunnerPythonUTF8Pipe(t *testing.T) {
	py, err := exec.LookPath("python")
	if err != nil || strings.Contains(strings.ToLower(py), "windowsapps") {
		t.Skip("python no disponible en PATH para probar salida UTF-8 por pipe")
	}
	origUTF8, hadUTF8 := os.LookupEnv("PYTHONUTF8")
	origIO, hadIO := os.LookupEnv("PYTHONIOENCODING")
	_ = os.Setenv("PYTHONUTF8", "0")
	_ = os.Setenv("PYTHONIOENCODING", "cp1252")
	t.Cleanup(func() {
		if hadUTF8 {
			_ = os.Setenv("PYTHONUTF8", origUTF8)
		} else {
			_ = os.Unsetenv("PYTHONUTF8")
		}
		if hadIO {
			_ = os.Setenv("PYTHONIOENCODING", origIO)
		} else {
			_ = os.Unsetenv("PYTHONIOENCODING")
		}
	})

	script := filepath.Join(t.TempDir(), "emit_utf8.py")
	code := "import sys\nprint('\\u2713 Validación Óptica OK')\nprint(f'enc={sys.stdout.encoding}')\n"
	if err := os.WriteFile(script, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}

	r := ShellRunner{}
	ev, err := r.Run(context.Background(), `"`+py+`" "`+script+`"`)
	if err != nil {
		t.Fatalf("error ejecutando script Python: %v", err)
	}
	if ev.ExitCode != 0 {
		t.Fatalf("ExitCode esperado 0, obtenido %d (output=%q)", ev.ExitCode, ev.Output)
	}
	if !strings.Contains(ev.Output, "✓") {
		t.Fatalf("falta '✓' en la evidencia capturada: %q", ev.Output)
	}
	if !strings.Contains(ev.Output, "Validación Óptica OK") {
		t.Fatalf("se perdieron tildes en la evidencia capturada: %q", ev.Output)
	}
}

func TestShellRunnerWaitDelay(t *testing.T) {
	r := ShellRunner{Timeout: 150 * time.Millisecond}
	cmd := "sleep 10"
	if filepath.Separator == '\\' {
		cmd = "ping -n 11 127.0.0.1"
	}
	start := time.Now()
	ev, err := r.Run(context.Background(), cmd)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("el timeout propio debe devolverse como evidencia, no error: %v", err)
	}
	if ev.ExitCode != -1 || !strings.Contains(ev.Output, "interrumpido por tiempo límite") {
		t.Fatalf("esperaba interrupción por timeout, ev=%+v", ev)
	}
	if elapsed > 6*time.Second {
		t.Fatalf("tardó demasiado en cerrar pipes/procesos (%v)", elapsed)
	}
}

type gitRepo struct {
	t   *testing.T
	dir string
}

func newGitRepo(t *testing.T) *gitRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no disponible")
	}
	dir := t.TempDir()
	// En Windows, Git crea objetos en .git/objects con atributo ReadOnly; lo quitamos antes del cleanup de TempDir.
	t.Cleanup(func() {
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && info != nil && !info.IsDir() {
				_ = os.Chmod(p, 0o666)
			}
			return nil
		})
	})
	g := &gitRepo{t: t, dir: dir}
	g.git("init", "-q")
	g.git("config", "user.email", "t@t.t")
	g.git("config", "user.name", "t")
	g.git("config", "commit.gpgsign", "false")
	return g
}

func (g *gitRepo) git(args ...string) string {
	g.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		g.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (g *gitRepo) write(name, content string) {
	g.t.Helper()
	p := filepath.Join(g.dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		g.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		g.t.Fatal(err)
	}
}

func (g *gitRepo) read(name string) string {
	b, err := os.ReadFile(filepath.Join(g.dir, name))
	if err != nil {
		return "<no existe>"
	}
	return string(b)
}

func TestGitWorkspaceCommitAndRevertKeepHistory(t *testing.T) {
	g := newGitRepo(t)
	g.write("a.txt", "v1")
	g.git("add", "-A")
	g.git("commit", "-q", "-m", "base")

	ws := GitWorkspace{Dir: g.dir}
	ctx := context.Background()
	base, err := ws.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if ch, err := ws.ChangedSince(ctx, base); err != nil || len(ch) != 0 {
		t.Fatalf("árbol limpio: %v %v", ch, err)
	}

	// el Chef modifica un archivo permitido y crea otro, y además toca uno que NO debe commitearse
	g.write("a.txt", "v2")
	g.write("nuevo/b.txt", "x")
	g.write("prohibido.txt", "no")
	ch, err := ws.ChangedSince(ctx, base)
	if err != nil || len(ch) != 3 {
		t.Fatalf("esperaba 3 cambios, got %v %v", ch, err)
	}

	ref, err := ws.Commit(ctx, "gengis: T1/A intento 1", []string{"a.txt", "nuevo/b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if ref == base {
		t.Fatal("el commit debe avanzar HEAD")
	}
	if got := g.git("log", "-1", "--format=%s"); got != "gengis: T1/A intento 1" {
		t.Fatalf("mensaje del commit: %q", got)
	}
	if st := g.git("status", "--porcelain"); !strings.Contains(st, "prohibido.txt") || strings.Contains(st, "a.txt") {
		t.Fatalf("solo lo permitido debía commitearse; estado: %q", st)
	}
	// ChangedSince sigue viendo los cambios ya commiteados desde la base
	if ch, _ := ws.ChangedSince(ctx, base); len(ch) != 3 {
		t.Fatalf("debe ver cambios commiteados y pendientes: %v", ch)
	}
	// sin cambios pendientes de esos archivos no crea un commit vacío
	ref2, err := ws.Commit(ctx, "otro", []string{"a.txt"})
	if err != nil || ref2 != ref {
		t.Fatalf("no debía crear un commit vacío: %v %s %s", err, ref2, ref)
	}

	before := g.git("rev-list", "--count", "HEAD")
	if err := ws.Revert(ctx, base); err != nil {
		t.Fatal(err)
	}
	if g.read("a.txt") != "v1" || g.read("nuevo/b.txt") != "<no existe>" {
		t.Fatalf("la reversión debe devolver el contenido de la base: a=%q b=%q", g.read("a.txt"), g.read("nuevo/b.txt"))
	}
	// el historial NO se destruye: hay más commits que antes (el de reversión), no menos
	if after := g.git("rev-list", "--count", "HEAD"); after <= before {
		t.Fatalf("el historial debe crecer con git revert (antes %s, después %s)", before, after)
	}
	// y el original sigue accesible
	if g.git("cat-file", "-t", ref) != "commit" {
		t.Fatal("el commit revertido debe seguir existiendo en el historial")
	}
	// lo que quedó sin commitear no se pierde: va al stash
	if stash := g.git("stash", "list"); !strings.Contains(stash, "gengiswap") {
		t.Fatalf("los cambios sin commitear debían guardarse en el stash: %q", stash)
	}
	if g.read("prohibido.txt") != "<no existe>" {
		t.Fatal("tras revertir, el árbol debe quedar limpio")
	}
}

func TestGitWorkspaceRetryScopeViolationCleansTree(t *testing.T) {
	g := newGitRepo(t)
	g.write("src/init.go", "package src\n")
	g.git("add", "-A")
	g.git("commit", "-q", "-m", "base")

	eng := &Engine{
		Prep: &fakePrep{chunks: mk("A")},
		Chef: fnChef{fn: func(_ context.Context, in ChefInput) (string, error) {
			if in.Attempt == 1 {
				g.write("src/ok.go", "package src\n")
				g.write("docs/prohibido.md", "fuera de alcance\n")
			}
			return "ok", nil
		}},
		Aud:    &fakeAud{plan: PlanVerdict{Approved: true}, script: []Verdict{{Decision: RetryChunk, Revert: false}, v(GoNext)}},
		Runner: okRunner(),
		WS:     GitWorkspace{Dir: g.dir},
		Store:  &MemStore{},
	}
	st, err := eng.Run(context.Background(), Task{ID: "T1", Prompt: "p"}, Scope{CanModify: []string{"src"}}, 3)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if st.Status != "DONE" {
		t.Fatalf("status=%s", st.Status)
	}
	if g.read("src/ok.go") != "package src\n" {
		t.Fatalf("el archivo permitido commiteado debía conservarse: %q", g.read("src/ok.go"))
	}
	if g.read("docs/prohibido.md") != "<no existe>" {
		t.Fatalf("el archivo prohibido debía haberse limpiado/stasheado: %q", g.read("docs/prohibido.md"))
	}
	if status := g.git("status", "--porcelain"); status != "" {
		t.Fatalf("el árbol debía quedar limpio, status=%q", status)
	}
	if stash := g.git("stash", "list"); !strings.Contains(stash, "gengiswap") {
		t.Fatalf("el archivo prohibido debía quedar en stash: %q", stash)
	}
}

func TestGitWorkspaceRevertConflictAbortsCleanly(t *testing.T) {
	g := newGitRepo(t)
	g.write("a.txt", "root\n")
	g.git("add", "-A")
	g.git("commit", "-q", "-m", "root")
	g.git("branch", "-M", "main")

	g.git("checkout", "-q", "-b", "side")
	g.write("side.txt", "side\n")
	g.git("add", "-A")
	g.git("commit", "-q", "-m", "side commit")

	g.git("checkout", "-q", "main")
	g.write("base.txt", "base\n")
	g.git("add", "-A")
	g.git("commit", "-q", "-m", "base commit")

	ws := GitWorkspace{Dir: g.dir}
	ctx := context.Background()
	base, err := ws.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}

	g.git("merge", "--no-ff", "--no-edit", "side")
	g.write("after.txt", "after\n")
	g.git("add", "-A")
	g.git("commit", "-q", "-m", "after merge")

	beforeHead, err := ws.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Revert(ctx, base); err == nil {
		t.Fatal("esperaba error al revertir un rango que contiene un commit merge")
	}
	afterHead, err := ws.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterHead != beforeHead {
		t.Fatalf("git revert --abort debía restaurar HEAD (%s), quedó en %s", beforeHead, afterHead)
	}
	for _, marker := range []string{"REVERT_HEAD", "sequencer"} {
		if _, err := os.Stat(filepath.Join(g.dir, ".git", marker)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf(".git/%s no debía permanecer tras abortar (err=%v)", marker, err)
		}
	}
	if status := g.git("status", "--porcelain"); status != "" {
		t.Fatalf("el árbol debía quedar limpio tras git revert --abort, status=%q", status)
	}
}

func TestGitWorkspaceRevertNothingToDo(t *testing.T) {
	g := newGitRepo(t)
	g.write("a.txt", "v1")
	g.git("add", "-A")
	g.git("commit", "-q", "-m", "base")
	ws := GitWorkspace{Dir: g.dir}
	base, _ := ws.Head(context.Background())
	if err := ws.Revert(context.Background(), base); err != nil {
		t.Fatalf("revertir sin cambios no debe fallar: %v", err)
	}
	if err := ws.Revert(context.Background(), ""); err != nil {
		t.Fatalf("base vacía es un no-op: %v", err)
	}
}
