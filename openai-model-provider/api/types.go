package api

type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID       string            `json:"id"`
	Object   string            `json:"object"`
	Created  int               `json:"created"`
	OwnedBy  string            `json:"owned_by"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
