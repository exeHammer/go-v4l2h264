package main

// Timeout error
type Timeout struct{}

func (e *Timeout) Error() string {
	return "Timeout occured"
}
