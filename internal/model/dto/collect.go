package dto

// Tweet represents a parsed tweet from the X API stream.
type Tweet struct {
	ID           string `json:"id"`
	Text         string `json:"text"`
	AuthorID     string `json:"author_id"`
	AuthorName   string `json:"author_name,omitempty"`
	AuthorHandle string `json:"author_handle,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

// StreamRule represents a Filtered Stream rule.
type StreamRule struct {
	ID    string `json:"id,omitempty"`
	Value string `json:"value"`
	Tag   string `json:"tag,omitempty"`
}
