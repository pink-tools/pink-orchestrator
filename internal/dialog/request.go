package dialog

// Request represents a dialog request from a service
type Request struct {
	Title         string `json:"title"`
	Message       string `json:"message"`
	ConfirmButton string `json:"confirm_button"`
	CancelButton  string `json:"cancel_button"`
}
