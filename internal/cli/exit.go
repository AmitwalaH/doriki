package cli

import (
	"errors"
	"strconv"
)

// Exit codes from DESIGN.md section 7.3. 3 arrives with --require-pack.
const (
	ExitIncomplete  = 1  // report produced with an expected phase open or absent (decision 0003)
	ExitUnavailable = 2  // pod not found, no permission, or the cluster cannot be reached
	ExitUsage       = 64 // any error that carries no code of its own
)

// ExitError ends the command with a specific exit code. Err, when set, is printed; an
// incomplete report has already been printed in full, so it has no message.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return "exit code " + strconv.Itoa(e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// Exit maps the error from Execute to the process exit code and the message main prints
// ("" for none).
func Exit(err error) (code int, msg string) {
	if err == nil {
		return 0, ""
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		if exit.Err == nil {
			return exit.Code, ""
		}
		return exit.Code, exit.Err.Error()
	}
	return ExitUsage, err.Error()
}
