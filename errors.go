package main

// error that notifies the handler that error occurred, but the pod should be created
type nonFatalError struct {
	err     error
	message string
}

func newNonFatalError(err error, message string) error {
	return &nonFatalError{
		err:     err,
		message: message,
	}
}

func (e *nonFatalError) Error() string {
	return e.err.Error()
}
