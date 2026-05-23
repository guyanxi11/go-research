package researcher

// Options tunes Phase 4 ReAct search and Critic retries.
type Options struct {
	// MaxSearchRounds is how many search waves to run (initial + follow-ups). Default 2.
	MaxSearchRounds int
	// MaxFollowUpQueries caps extra queries per follow-up wave. Default 2.
	MaxFollowUpQueries int

	CriticEnabled    bool
	CriticMinScore   int // default 6
	MaxCriticRetries int // extra attempts after a failing review; default 1
}

func (o Options) normalized() Options {
	if o.MaxSearchRounds <= 0 {
		o.MaxSearchRounds = 2
	}
	if o.MaxFollowUpQueries <= 0 {
		o.MaxFollowUpQueries = 2
	}
	if o.CriticMinScore <= 0 {
		o.CriticMinScore = 6
	}
	if o.MaxCriticRetries < 0 {
		o.MaxCriticRetries = 0
	}
	return o
}

// ProgressHook receives live updates for SSE bridging (optional).
type ProgressHook func(ev ProgressEvent)

// ProgressEvent is emitted during Research for orchestrator / UI.
type ProgressEvent struct {
	SearchRound  *SearchRoundPayload  `json:"search_round,omitempty"`
	CriticReview *CriticReviewPayload `json:"critic_review,omitempty"`
}

type SearchRoundPayload struct {
	TaskID      string `json:"task_id"`
	Round       int    `json:"round"`
	Query       string `json:"query"`
	ResultCount int    `json:"result_count"`
}

type CriticReviewPayload struct {
	TaskID   string `json:"task_id"`
	Attempt  int    `json:"attempt"`
	Score    int    `json:"score"`
	Pass     bool   `json:"pass"`
	Feedback string `json:"feedback,omitempty"`
}
