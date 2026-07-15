package agents

type Locale string

const (
	LocaleEnglish  Locale = "en"
	LocaleChinese  Locale = "zh"
	LocaleJapanese Locale = "ja"
)

type AgentMeta struct {
	Avatar      string   `json:"avatar"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	SystemRole  string   `json:"systemRole,omitempty"`
}

type AgentConfig struct {
	SystemRole string `json:"systemRole,omitempty"`
}

type Agent struct {
	Identifier string       `json:"identifier"`
	Meta       AgentMeta    `json:"meta"`
	CreatedAt  string       `json:"createdAt"`
	Homepage   string       `json:"homepage"`
	Author     string       `json:"author"`
	IsCustom   bool         `json:"isCustom,omitempty"`
	Config     *AgentConfig `json:"config,omitempty"`
}

type ListResponse struct {
	Agents      []Agent `json:"agents"`
	Unavailable bool    `json:"unavailable,omitempty"`
}
