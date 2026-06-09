package handlers

// httpStatusError is an error that carries an HTTP status code so that
// NewError maps it to the right response code rather than defaulting to 500.
type httpStatusError struct {
	code int
	msg  string
}

func (e *httpStatusError) Error() string { return e.msg }
func (e *httpStatusError) Code() int     { return e.code }
