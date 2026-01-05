package main

import "fmt"

type KeyNotFoundError struct {
	Key any
}

func (e *KeyNotFoundError) Error() string {
	return fmt.Sprintf("key not found: %v", e.Key)
}
