package integration

type ItemDetails struct {
	Title string `json:"title"`
	State string `json:"state"`
	URL   string `json:"url"`
}

type ExternalDetailsResult struct {
	Details *ItemDetails `json:"details"`
	Error   *string      `json:"error,omitempty"`
}

type Integration interface {
	ValidateConfig(config map[string]interface{}) error
	FetchItemDetails(config map[string]interface{}, itemID string) (*ItemDetails, error)
}
