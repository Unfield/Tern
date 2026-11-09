package kit

import (
	"errors"
	"fmt"
	"net/http"
)

// HTTPError is an expected error with an HTTP status, such as a 404.
// Its message is shown to the user, so it must not contain secrets.
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("%d %s", e.Status, e.Message) }

// Error returns an expected error with the given status and a message that
// is safe to show to users. Return it from Load, an action or an endpoint
// to render the nearest +error.svelte.
//
// Any other error returned from your code is treated as unexpected: it is
// logged, and users see only "Internal Error".
func Error(status int, message string) error {
	if status < 400 || status > 599 {
		panic(fmt.Sprintf("kit.Error: status %d is not an error status (400-599)", status))
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return &HTTPError{Status: status, Message: message}
}

// NotFound returns a 404 error.
func NotFound() error { return Error(http.StatusNotFound, "") }

// RedirectError tells Tern to redirect the request.
type RedirectError struct {
	Status   int
	Location string
}

func (e *RedirectError) Error() string { return fmt.Sprintf("redirect %d to %s", e.Status, e.Location) }

// Redirect returns an error that redirects to location with a 3xx status.
// Use 303 after a form submission, 307 or 308 to keep the request method,
// and 301 or 302 for permanent and temporary moves.
func Redirect(status int, location string) error {
	if status < 300 || status > 308 {
		panic(fmt.Sprintf("kit.Redirect: status %d is not a redirect status (300-308)", status))
	}
	return &RedirectError{Status: status, Location: location}
}

// FailError is returned by an action to report a failed submission,
// typically a validation error. Its Data is sent to the page as `form`.
type FailError struct {
	Status int
	Data   any
}

func (e *FailError) Error() string { return fmt.Sprintf("action failed with status %d", e.Status) }

// Fail returns an action failure with a 4xx status. The page re-renders with
// data available as the `form` prop, so it can show errors next to fields
// and keep the user's input.
func Fail(status int, data any) error {
	if status < 400 || status > 499 {
		panic(fmt.Sprintf("kit.Fail: status %d is not a client error status (400-499)", status))
	}
	return &FailError{Status: status, Data: data}
}

// classify maps an error from user code to a status and a public message.
func classify(err error) (status int, message string, expected bool) {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Status, he.Message, true
	}
	return http.StatusInternalServerError, "Internal Error", false
}
