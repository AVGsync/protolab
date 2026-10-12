package spec

// Issue is a problem found in a spec; Path is a JSONPath-like location ("$.messages[2].fields[0].type").
type Issue struct {
	Path     string `json:"path"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // "error" | "warning"
}

// ValidationResult is returned by POST /interpretations/validate.
type ValidationResult struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}

// Parse decodes raw JSON strictly and validates it. The spec is nil when it could not be decoded.
// Issues is never nil.
func Parse(raw []byte) (*Spec, ValidationResult) { return parse(raw) }

// Validate checks semantic rules of a decoded spec. It never returns nil.
func Validate(s *Spec) []Issue { return validate(s, nil) }
