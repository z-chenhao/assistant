package observability

// Trace identity follows an execution boundary: a Main Run or a Root epoch.
// A session groups related traces; it never changes domain Run ancestry.
type TraceQuery struct {
	ExecutionQuery
	Session string
}

func (q TraceQuery) Validate() error {
	if len(q.Session) > 128 {
		return ErrInvalid
	}
	return q.ExecutionQuery.Validate()
}

type Metrics struct {
	Generations int64  `json:"generations"`
	KnownUsage  int64  `json:"known_usage"`
	Input       int64  `json:"input_tokens"`
	Output      int64  `json:"output_tokens"`
	CacheRead   int64  `json:"cache_read_tokens"`
	CacheWrite  int64  `json:"cache_write_tokens"`
	ProviderMS  *int64 `json:"provider_ms,omitempty"`
	Costs       []Cost `json:"costs"`
	Unpriced    int64  `json:"unpriced"`
}
type Cost struct {
	Currency string `json:"currency"`
	Micros   int64  `json:"micros"`
}
type Trace struct {
	Execution
	Session string  `json:"session_id"`
	Title   string  `json:"title"`
	Model   string  `json:"model"`
	Input   string  `json:"input_preview"`
	Output  string  `json:"output_preview"`
	Metrics Metrics `json:"metrics"`
}
type Traces struct {
	Items []Trace `json:"items"`
	Next  string  `json:"next,omitempty"`
}

type ObservationQuery struct {
	RunID, Cursor, Selected string
}

func (q ObservationQuery) Validate() error {
	if q.RunID == "" || len(q.RunID) > 128 || len(q.Cursor) > 512 || len(q.Selected) > 300 {
		return ErrInvalid
	}
	return nil
}

type Observation struct {
	ID      string `json:"id"`
	Parent  string `json:"parent_id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	RunID   string `json:"run_id"`
	Key     string `json:"key"`
	Step    *int64 `json:"step,omitempty"`
	Status  string `json:"status"`
	Started int64  `json:"started_at_us"`
	Ended   *int64 `json:"ended_at_us,omitempty"`
	Timing  string `json:"timing"`
	Detail  string `json:"detail,omitempty"`
}
type ObservationTree struct {
	Trace    Trace         `json:"trace"`
	Items    []Observation `json:"items"`
	Selected Observation   `json:"selected"`
	Next     string        `json:"next,omitempty"`
}
type BodyQuery struct {
	ObservationQuery
	Field  string
	Offset int
}

func (q BodyQuery) Validate() error {
	if q.ObservationQuery.Validate() != nil || q.Selected == "" || q.Offset < 0 || q.Offset > 1<<30 {
		return ErrInvalid
	}
	switch q.Field {
	case "input", "output", "wire":
		return nil
	}
	return ErrInvalid
}

type Body struct {
	Content   string `json:"content"`
	Length    int    `json:"length"`
	Next      *int   `json:"next_offset,omitempty"`
	Available bool   `json:"available"`
	Note      string `json:"note,omitempty"`
}
type ChatPreview struct {
	Index  int    `json:"index"`
	Role   string `json:"role"`
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
	CallID string `json:"tool_call_id,omitempty"`
	Calls  string `json:"tool_calls,omitempty"`
	More   bool   `json:"more"`
}
