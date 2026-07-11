package dto

// PageRequest is the basic pagination request query parameters.
type PageRequest struct {
	Page     int `json:"page" form:"page" binding:"min=1"`
	PageSize int `json:"page_size" form:"page_size" binding:"min=1,max=100"`
}

// IDRequest is a request carrying a single entity ID.
type IDRequest struct {
	ID int64 `json:"id" uri:"id" binding:"required,min=1"`
}

// DeleteRequest is a request to delete an entity.
type DeleteRequest struct {
	ID int64 `json:"id" form:"id" binding:"required,min=1"`
}
