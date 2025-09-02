package corrector

type CorrectorConfig struct {
	MaxEditDistance  int
	FreqTemperature  float64
	TopKSuggestions  int
	BetaWeight       float64
	LambdaPenalty    float64
	GammaMorph       float64
	MarginThreshold  float64
	TauInVocab       float64
	TauOutVocab      float64
	UseSymSpell      bool
	UseMorphology    bool
	EnableContext    bool
	FilterShortWords bool
	TransposeCost    float64
	NeighborInsDel   float64
	KeyboardNearSub  float64
}

type Candidate struct {
	Term  string
	Cost  float64
	Score float64
	Edits int
}

type SuggestionInfo struct {
	Token       string   `json:"token"`
	Suggestions []string `json:"suggestions"`
	Decision    string   `json:"decision"`
}

type CorrectionResult struct {
	Original     string                 `json:"original"`
	Corrected    string                 `json:"corrected"`
	Alternatives []string               `json:"alternatives,omitempty"`
	Suggestions  map[int]SuggestionInfo `json:"suggestions"`
}
