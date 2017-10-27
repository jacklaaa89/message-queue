package store

import (
	"encoding/json"
	"net/http"
)

// api an encapsulation of an api
// these functions are separated from the server
// as they provide analytics on the store rather
// which are not a core part of the server functionality.
type api struct {
	// store reference to the store.
	store Store
}

// countResponse a response from a count operation.
type countResponse struct {
	Count      int64 `json:"count"`
	StatusCode int   `json:"status_code"`
	Error      error `json:"error,omitempty"`
}

// count gets an estimated count of all the items in the cache.
// this is a simple demonstration that we can perform analytical operations.
func (a *api) count(w http.ResponseWriter, r *http.Request) {
	c, err := a.store.Count()
	var response = countResponse{StatusCode: StatusOK, Count: c, Error: err}
	if err != nil {
		response.StatusCode = StatusErr
	}

	data, _ := json.Marshal(response)

	w.Write(data)
}

// newApi initialises a new api instance attaching the store instance.
func newApi(store Store) api {
	return api{store: store}
}
