package server

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/AVGsync/protolab/internal/app"
	"github.com/AVGsync/protolab/internal/model"
)

func (s *server) routes(mux *http.ServeMux) {
	h := func(pattern string, fn func(w http.ResponseWriter, r *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := fn(w, r); err != nil {
				s.fail(w, r, err)
			}
		})
	}
	const v1 = "/api/v1"

	// Projects
	h("GET "+v1+"/projects", s.listProjects)
	h("POST "+v1+"/projects", s.createProject)
	h("POST "+v1+"/projects/import", s.importProject)
	h("POST "+v1+"/demo", s.createDemo)
	h("GET "+v1+"/projects/{projectId}", s.getProject)
	h("PATCH "+v1+"/projects/{projectId}", s.updateProject)
	h("GET "+v1+"/projects/{projectId}/export", s.exportProject)

	// Captures and sessions
	h("GET "+v1+"/projects/{projectId}/captures", s.listCaptures)
	h("POST "+v1+"/projects/{projectId}/captures", s.uploadCapture)
	h("GET "+v1+"/captures/{captureId}", s.getCapture)
	h("GET "+v1+"/projects/{projectId}/sessions", s.listSessions)
	h("GET "+v1+"/sessions/{sessionId}", s.getSession)
	h("GET "+v1+"/sessions/{sessionId}/packets", s.packets)
	h("GET "+v1+"/sessions/{sessionId}/streams/{dir}", s.stream)
	h("GET "+v1+"/sessions/{sessionId}/streams/{dir}/bytes", s.streamBytes)
	h("GET "+v1+"/sessions/{sessionId}/messages", s.sessionMessages)
	h("GET "+v1+"/projects/{projectId}/messages", s.projectMessages)

	// Action log
	h("GET "+v1+"/projects/{projectId}/actions", s.listActions)
	h("POST "+v1+"/projects/{projectId}/actions", s.createAction)
	h("POST "+v1+"/projects/{projectId}/actions/import", s.importActions)

	// Interpretations
	h("GET "+v1+"/projects/{projectId}/interpretations", s.listInterpretations)
	h("POST "+v1+"/projects/{projectId}/interpretations", s.createInterpretation)
	h("POST "+v1+"/interpretations/validate", s.validateSpec)
	h("POST "+v1+"/interpretations/preview", s.previewSpec)
	h("GET "+v1+"/interpretations/{id}", s.getInterpretation)
	h("GET "+v1+"/interpretations/{id}/versions", s.listVersions)
	h("POST "+v1+"/interpretations/{id}/versions", s.createVersion)
	h("GET "+v1+"/interpretation-versions/{versionId}", s.getVersion)
	h("GET "+v1+"/interpretation-versions/{versionId}/export", s.exportVersion)

	// Hypotheses and observations
	h("GET "+v1+"/projects/{projectId}/hypotheses", s.listHypotheses)
	h("POST "+v1+"/projects/{projectId}/hypotheses", s.createHypothesis)
	h("PATCH "+v1+"/hypotheses/{id}", s.updateHypothesis)
	h("GET "+v1+"/projects/{projectId}/observations", s.listObservations)
	h("POST "+v1+"/projects/{projectId}/observations", s.createObservation)
	h("DELETE "+v1+"/observations/{id}", s.deleteObservation)

	// Verification
	h("GET "+v1+"/projects/{projectId}/corpora", s.listCorpora)
	h("POST "+v1+"/projects/{projectId}/corpora", s.createCorpus)
	h("GET "+v1+"/projects/{projectId}/runs", s.listRuns)
	h("POST "+v1+"/projects/{projectId}/runs", s.startRun)
	h("GET "+v1+"/runs/{runId}", s.getRun)
	h("GET "+v1+"/runs/{runId}/findings", s.listFindings)

	// Reports, jobs, system
	h("GET "+v1+"/projects/{projectId}/reports", s.listReports)
	h("POST "+v1+"/projects/{projectId}/reports", s.createReport)
	h("GET "+v1+"/reports/{reportId}/files/{fileName}", s.reportFile)
	h("GET "+v1+"/jobs", s.listJobs)
	h("GET "+v1+"/jobs/{jobId}", s.getJob)
	h("POST "+v1+"/jobs/{jobId}/cancel", s.cancelJob)
	h("GET "+v1+"/system/info", s.systemInfo)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Неизвестный метод API: "+r.Method+" "+r.URL.Path, nil)
	})
}

func list[T any](w http.ResponseWriter, items []T, err error) error {
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, model.List(items))
	return nil
}

func one(w http.ResponseWriter, status int, v any, err error) error {
	if err != nil {
		return err
	}
	writeJSON(w, status, v)
	return nil
}

func attachment(w http.ResponseWriter, name, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
}

// uploadedFile returns the "file" part of a multipart request without buffering it.
func uploadedFile(r *http.Request) (string, io.Reader, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return "", nil, &app.Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "Ожидается multipart/form-data с полем file"}
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return "", nil, &app.Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "Файл не передан (поле file)"}
		}
		if err != nil {
			return "", nil, &app.Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "Некорректный multipart: " + err.Error()}
		}
		if part.FormName() == "file" {
			return part.FileName(), part, nil
		}
		part.Close()
	}
}

func queryInt(r *http.Request, key string, def int) (int, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, &app.Error{Status: http.StatusBadRequest, Code: "bad_request", Message: "Параметр " + key + " должен быть неотрицательным целым"}
	}
	return n, nil
}

// ------------------------------------------------------------------ projects

func (s *server) listProjects(w http.ResponseWriter, r *http.Request) error {
	return list(w, s.app.ListProjects(), nil)
}

func (s *server) createProject(w http.ResponseWriter, r *http.Request) error {
	var in model.CreateProjectInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	p, err := s.app.CreateProject(in)
	return one(w, http.StatusCreated, p, err)
}

func (s *server) createDemo(w http.ResponseWriter, r *http.Request) error {
	p, err := s.app.CreateDemoProject(r.Context())
	return one(w, http.StatusCreated, p, err)
}

func (s *server) importProject(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, s.app.Limits().MaxCaptureBytes*8+(1<<20))
	name, file, err := uploadedFile(r)
	if err != nil {
		return err
	}
	p, _, err := s.app.ImportProject(name, file)
	return one(w, http.StatusCreated, p, err)
}

func (s *server) getProject(w http.ResponseWriter, r *http.Request) error {
	p, err := s.app.GetProject(r.PathValue("projectId"))
	return one(w, http.StatusOK, p, err)
}

func (s *server) updateProject(w http.ResponseWriter, r *http.Request) error {
	var patch app.ProjectPatch
	if err := decode(w, r, &patch); err != nil {
		return err
	}
	p, err := s.app.UpdateProject(r.PathValue("projectId"), patch)
	return one(w, http.StatusOK, p, err)
}

func (s *server) exportProject(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("projectId")
	if _, err := s.app.GetProject(id); err != nil {
		return err
	}
	attachment(w, s.app.ExportFileName(id), "application/zip")
	if err := s.app.ExportProject(id, w); err != nil {
		// Headers are already sent; the truncated archive is detected by the client.
		s.log.Error("export failed", "project", id, "err", err)
	}
	return nil
}

// ------------------------------------------------------------------ captures & sessions

func (s *server) listCaptures(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.ListCaptures(r.PathValue("projectId"))
	return list(w, items, err)
}

func (s *server) uploadCapture(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, s.app.Limits().MaxCaptureBytes+(1<<20))
	name, file, err := uploadedFile(r)
	if err != nil {
		return err
	}
	res, err := s.app.ImportCapture(r.PathValue("projectId"), name, file)
	return one(w, http.StatusAccepted, res, err)
}

func (s *server) getCapture(w http.ResponseWriter, r *http.Request) error {
	c, err := s.app.GetCapture(r.PathValue("captureId"))
	return one(w, http.StatusOK, c, err)
}

func (s *server) listSessions(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	f := app.SessionFilter{CaptureID: q.Get("captureId"), Status: q.Get("status"), Query: q.Get("q")}
	if t := q.Get("target"); t != "" {
		b := t == "true"
		f.Target = &b
	}
	items, err := s.app.ListSessions(r.PathValue("projectId"), f)
	return list(w, items, err)
}

func (s *server) getSession(w http.ResponseWriter, r *http.Request) error {
	v, err := s.app.GetSession(r.PathValue("sessionId"))
	return one(w, http.StatusOK, v, err)
}

func (s *server) packets(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.Packets(r.PathValue("sessionId"))
	return list(w, items, err)
}

func (s *server) stream(w http.ResponseWriter, r *http.Request) error {
	v, err := s.app.Stream(r.PathValue("sessionId"), model.Direction(r.PathValue("dir")))
	return one(w, http.StatusOK, v, err)
}

func (s *server) streamBytes(w http.ResponseWriter, r *http.Request) error {
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		return err
	}
	length, err := queryInt(r, "length", -1)
	if err != nil && r.URL.Query().Get("length") != "" {
		return err
	}
	data, total, err := s.app.StreamBytes(r.PathValue("sessionId"), model.Direction(r.PathValue("dir")), offset, length)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("X-Stream-Length", strconv.Itoa(total))
	_, _ = w.Write(data)
	return nil
}

func (s *server) sessionMessages(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.SessionMessages(r.PathValue("sessionId"), r.URL.Query().Get("versionId"))
	return list(w, items, err)
}

func (s *server) projectMessages(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	mq := app.MessagesQuery{VersionID: q.Get("versionId"), RuleID: q.Get("ruleId"), Direction: q.Get("direction")}
	if ids := q.Get("sessionIds"); ids != "" {
		mq.SessionIDs = strings.Split(ids, ",")
	}
	items, err := s.app.ProjectMessages(r.PathValue("projectId"), mq)
	return list(w, items, err)
}

// ------------------------------------------------------------------ actions

func (s *server) listActions(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.ListActions(r.PathValue("projectId"))
	return list(w, items, err)
}

func (s *server) createAction(w http.ResponseWriter, r *http.Request) error {
	var in model.CreateActionInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	a, err := s.app.CreateAction(r.PathValue("projectId"), in)
	return one(w, http.StatusCreated, a, err)
}

func (s *server) importActions(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 17<<20)
	name, file, err := uploadedFile(r)
	if err != nil {
		return err
	}
	res, err := s.app.ImportActions(r.PathValue("projectId"), name, file)
	return one(w, http.StatusOK, res, err)
}

// ------------------------------------------------------------------ interpretations

func (s *server) listInterpretations(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.ListInterpretations(r.PathValue("projectId"))
	return list(w, items, err)
}

func (s *server) createInterpretation(w http.ResponseWriter, r *http.Request) error {
	var in app.CreateInterpretationInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	v, err := s.app.CreateInterpretation(r.PathValue("projectId"), in)
	return one(w, http.StatusCreated, v, err)
}

func (s *server) getInterpretation(w http.ResponseWriter, r *http.Request) error {
	v, err := s.app.GetInterpretation(r.PathValue("id"))
	return one(w, http.StatusOK, v, err)
}

func (s *server) listVersions(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.ListVersions(r.PathValue("id"))
	return list(w, items, err)
}

func (s *server) createVersion(w http.ResponseWriter, r *http.Request) error {
	var in app.CreateVersionInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	v, err := s.app.CreateVersion(r.PathValue("id"), in)
	return one(w, http.StatusCreated, v, err)
}

func (s *server) getVersion(w http.ResponseWriter, r *http.Request) error {
	v, err := s.app.GetVersion(r.PathValue("versionId"))
	return one(w, http.StatusOK, v, err)
}

func (s *server) exportVersion(w http.ResponseWriter, r *http.Request) error {
	data, name, err := s.app.ExportVersion(r.PathValue("versionId"))
	if err != nil {
		return err
	}
	attachment(w, name, "application/json")
	_, _ = w.Write(data)
	return nil
}

func (s *server) validateSpec(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Spec rawJSON `json:"spec"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	return one(w, http.StatusOK, s.app.ValidateSpec([]byte(in.Spec)), nil)
}

func (s *server) previewSpec(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Spec      rawJSON `json:"spec"`
		SessionID string  `json:"sessionId"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	items, err := s.app.PreviewSpec([]byte(in.Spec), in.SessionID)
	return list(w, items, err)
}

// rawJSON keeps a JSON value verbatim (like json.RawMessage, but "null" stays empty).
type rawJSON []byte

func (m *rawJSON) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*m = nil
		return nil
	}
	*m = append((*m)[:0], b...)
	return nil
}

// ------------------------------------------------------------------ hypotheses & observations

func (s *server) listHypotheses(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.ListHypotheses(r.PathValue("projectId"))
	return list(w, items, err)
}

func (s *server) createHypothesis(w http.ResponseWriter, r *http.Request) error {
	var in model.CreateHypothesisInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	h, err := s.app.CreateHypothesis(r.PathValue("projectId"), in)
	return one(w, http.StatusCreated, h, err)
}

func (s *server) updateHypothesis(w http.ResponseWriter, r *http.Request) error {
	var in model.UpdateHypothesisInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	h, err := s.app.UpdateHypothesis(r.PathValue("id"), in)
	return one(w, http.StatusOK, h, err)
}

func (s *server) listObservations(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	items, err := s.app.ListObservations(r.PathValue("projectId"), app.ObservationFilter{HypothesisID: q.Get("hypothesisId"), SessionID: q.Get("sessionId")})
	return list(w, items, err)
}

func (s *server) createObservation(w http.ResponseWriter, r *http.Request) error {
	var in model.CreateObservationInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	o, err := s.app.CreateObservation(r.PathValue("projectId"), in)
	return one(w, http.StatusCreated, o, err)
}

func (s *server) deleteObservation(w http.ResponseWriter, r *http.Request) error {
	if err := s.app.DeleteObservation(r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ------------------------------------------------------------------ verification

func (s *server) listCorpora(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.ListCorpora(r.PathValue("projectId"))
	return list(w, items, err)
}

func (s *server) createCorpus(w http.ResponseWriter, r *http.Request) error {
	var in model.CreateCorpusInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	c, err := s.app.CreateCorpus(r.PathValue("projectId"), in)
	return one(w, http.StatusCreated, c, err)
}

func (s *server) listRuns(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.ListRuns(r.PathValue("projectId"))
	return list(w, items, err)
}

func (s *server) startRun(w http.ResponseWriter, r *http.Request) error {
	var in model.StartRunInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	res, err := s.app.StartRun(r.PathValue("projectId"), in)
	return one(w, http.StatusAccepted, res, err)
}

func (s *server) getRun(w http.ResponseWriter, r *http.Request) error {
	v, err := s.app.GetRun(r.PathValue("runId"))
	return one(w, http.StatusOK, v, err)
}

func (s *server) listFindings(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.ListFindings(r.PathValue("runId"), r.URL.Query().Get("kind"))
	return list(w, items, err)
}

// ------------------------------------------------------------------ reports, jobs, system

func (s *server) listReports(w http.ResponseWriter, r *http.Request) error {
	items, err := s.app.ListReports(r.PathValue("projectId"))
	return list(w, items, err)
}

func (s *server) createReport(w http.ResponseWriter, r *http.Request) error {
	var in model.CreateReportInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	rep, jobID, err := s.app.CreateReport(r.PathValue("projectId"), in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"report": rep, "jobId": jobID})
	return nil
}

func (s *server) reportFile(w http.ResponseWriter, r *http.Request) error {
	name := r.PathValue("fileName")
	path, ctype, err := s.app.ReportFile(r.PathValue("reportId"), name)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	attachment(w, name, ctype)
	_, _ = io.Copy(w, f)
	return nil
}

func (s *server) listJobs(w http.ResponseWriter, r *http.Request) error {
	limit, err := queryInt(r, "limit", 20)
	if err != nil {
		return err
	}
	return list(w, s.app.Jobs().List(min(limit, 100)), nil)
}

func (s *server) getJob(w http.ResponseWriter, r *http.Request) error {
	j, err := s.app.Jobs().Get(r.PathValue("jobId"))
	return one(w, http.StatusOK, j, err)
}

func (s *server) cancelJob(w http.ResponseWriter, r *http.Request) error {
	j, err := s.app.Jobs().Cancel(r.PathValue("jobId"))
	return one(w, http.StatusOK, j, err)
}

func (s *server) systemInfo(w http.ResponseWriter, r *http.Request) error {
	return one(w, http.StatusOK, s.app.SystemInfo(), nil)
}
