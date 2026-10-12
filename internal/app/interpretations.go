package app

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/report"
	"github.com/AVGsync/protolab/internal/spec"
	"github.com/AVGsync/protolab/internal/store"
)

// withoutMeta drops the "$meta" object of an exported spec file (report.SpecFile),
// so a downloaded interpretation can be pasted back or loaded into another
// project, as `protolab apply --spec` accepts it.
func withoutMeta(raw json.RawMessage) json.RawMessage {
	if out, err := report.StripMeta(raw); err == nil {
		return out
	}
	return raw // not an object: the strict parser reports it
}

// checkSpec decodes and validates a spec; invalid specs give a 422 with the issues.
func checkSpec(raw json.RawMessage) (*spec.Spec, error) {
	if len(raw) == 0 {
		return nil, &Error{Status: http.StatusUnprocessableEntity, Code: "invalid_spec", Message: "Спецификация не передана", Details: []spec.Issue{{Path: "$", Message: "Спецификация не передана", Severity: "error"}}}
	}
	s, res := spec.Parse(withoutMeta(raw))
	if !res.Valid || s == nil {
		return nil, &Error{Status: http.StatusUnprocessableEntity, Code: "invalid_spec", Message: "Интерпретация содержит ошибки", Details: res.Issues}
	}
	return s, nil
}

func (a *App) ListInterpretations(projectID string) ([]model.Interpretation, error) {
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return nonNil(slices.Clone(p.st.Interpretations)), nil
}

func (a *App) GetInterpretation(id string) (model.Interpretation, error) {
	p, err := a.projectOf(id, "Интерпретация")
	if err != nil {
		return model.Interpretation{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	i := findIndex(p.st.Interpretations, func(x *model.Interpretation) bool { return x.ID == id })
	if i < 0 {
		return model.Interpretation{}, notFound("Интерпретация")
	}
	return p.st.Interpretations[i], nil
}

type CreateInterpretationInput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Spec        json.RawMessage `json:"spec"`
}

func (a *App) CreateInterpretation(projectID string, in CreateInterpretationInput) (model.Interpretation, error) {
	p, err := a.project(projectID)
	if err != nil {
		return model.Interpretation{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return model.Interpretation{}, invalid("Укажите название")
	}
	s, err := checkSpec(in.Spec)
	if err != nil {
		return model.Interpretation{}, err
	}
	return a.addInterpretation(p, name, strings.TrimSpace(in.Description), s, "Первая версия", "")
}

// addInterpretation creates an interpretation with its first version.
func (a *App) addInterpretation(p *project, name, description string, s *spec.Spec, comment, createdAt string) (model.Interpretation, error) {
	if createdAt == "" {
		createdAt = model.Now()
	}
	interp := model.Interpretation{
		ID:            newID("int"),
		ProjectID:     p.id,
		Name:          name,
		Description:   description,
		LatestVersion: 1,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
	v := model.InterpretationVersion{
		ID:               newID("ver"),
		InterpretationID: interp.ID,
		Version:          1,
		Hash:             spec.Hash(s),
		Comment:          comment,
		CreatedAt:        createdAt,
		Spec:             *s,
	}
	interp.LatestVersionID = v.ID
	err := p.update(func(st *store.State) error {
		st.Interpretations = append(st.Interpretations, interp)
		st.Versions = append(st.Versions, v)
		return nil
	})
	if err != nil {
		return model.Interpretation{}, err
	}
	a.index(p.id, interp.ID, v.ID)
	return interp, nil
}

func (a *App) ListVersions(interpretationID string) ([]model.InterpretationVersion, error) {
	p, err := a.projectOf(interpretationID, "Интерпретация")
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []model.InterpretationVersion
	for _, v := range p.st.Versions {
		if v.InterpretationID == interpretationID {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(x, y model.InterpretationVersion) int { return y.Version - x.Version })
	return nonNil(out), nil
}

type CreateVersionInput struct {
	Spec    json.RawMessage `json:"spec"`
	Comment string          `json:"comment"`
}

// CreateVersion stores a new immutable version. Older verification runs
// automatically become stale (see runDTO).
func (a *App) CreateVersion(interpretationID string, in CreateVersionInput) (model.InterpretationVersion, error) {
	p, err := a.projectOf(interpretationID, "Интерпретация")
	if err != nil {
		return model.InterpretationVersion{}, err
	}
	s, err := checkSpec(in.Spec)
	if err != nil {
		return model.InterpretationVersion{}, err
	}
	return a.addVersion(p, interpretationID, s, in.Comment, "")
}

func (a *App) addVersion(p *project, interpretationID string, s *spec.Spec, comment, createdAt string) (model.InterpretationVersion, error) {
	if createdAt == "" {
		createdAt = model.Now()
	}
	hash := spec.Hash(s)
	var v model.InterpretationVersion
	err := p.update(func(st *store.State) error {
		i := findIndex(st.Interpretations, func(x *model.Interpretation) bool { return x.ID == interpretationID })
		if i < 0 {
			return notFound("Интерпретация")
		}
		interp := &st.Interpretations[i]
		if j := findIndex(st.Versions, func(x *model.InterpretationVersion) bool { return x.ID == interp.LatestVersionID }); j >= 0 && st.Versions[j].Hash == hash {
			return conflict("no_changes", "Спецификация не отличается от последней версии")
		}
		comment = strings.TrimSpace(comment)
		if comment == "" {
			comment = "Без комментария"
		}
		v = model.InterpretationVersion{
			ID:               newID("ver"),
			InterpretationID: interp.ID,
			Version:          interp.LatestVersion + 1,
			Hash:             hash,
			Comment:          comment,
			CreatedAt:        createdAt,
			ParentVersionID:  interp.LatestVersionID,
			Spec:             *s,
		}
		st.Versions = append(st.Versions, v)
		interp.LatestVersion = v.Version
		interp.LatestVersionID = v.ID
		interp.UpdatedAt = createdAt
		return nil
	})
	if err != nil {
		return model.InterpretationVersion{}, err
	}
	a.index(p.id, v.ID)
	return v, nil
}

func (a *App) GetVersion(versionID string) (model.InterpretationVersion, error) {
	p, err := a.projectOf(versionID, "Версия")
	if err != nil {
		return model.InterpretationVersion{}, err
	}
	v, err := p.version(versionID)
	if err != nil {
		return model.InterpretationVersion{}, notFound("Версия")
	}
	return v, nil
}

// ExportVersion returns the spec as a standalone JSON file with metadata.
func (a *App) ExportVersion(versionID string) ([]byte, string, error) {
	v, err := a.GetVersion(versionID)
	if err != nil {
		return nil, "", err
	}
	interp, err := a.GetInterpretation(v.InterpretationID)
	if err != nil {
		return nil, "", err
	}
	data, err := report.SpecFile(v, interp.Name)
	if err != nil {
		return nil, "", err
	}
	_, name, _, _ := report.Names(interp.Name, v.Version)
	return data, name, nil
}

func (a *App) ValidateSpec(raw json.RawMessage) spec.ValidationResult {
	_, res := spec.Parse(withoutMeta(raw))
	if res.Issues == nil {
		res.Issues = []spec.Issue{}
	}
	return res
}

// PreviewSpec applies an unsaved spec to a session.
func (a *App) PreviewSpec(raw json.RawMessage, sessionID string) ([]model.Message, error) {
	s, err := checkSpec(raw)
	if err != nil {
		return nil, err
	}
	p, err := a.projectOfSession(sessionID)
	if err != nil {
		return nil, err
	}
	sd, err := p.session(sessionID)
	if err != nil {
		return nil, err
	}
	parsed := p.parse(sd, s, spec.Hash(s))
	if parsed.TooMany {
		return nil, sessionMessageLimit(a.cfg.Limits.MaxMessages)
	}
	return nonNil(parsed.Messages), nil
}
