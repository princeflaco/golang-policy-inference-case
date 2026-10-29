package apperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestNewFormatsMessage(t *testing.T) {
	err := New(InvalidInput, "input.%s: %s", "age", "bad")
	if err.Code != InvalidInput || err.Error() != "input.age: bad" {
		t.Fatalf("New() = %+v", err)
	}
}

func TestFromUnwrapsCodedErrors(t *testing.T) {
	coded := New(InvalidPolicy, "no start")
	if got := From(fmt.Errorf("parse: %w", coded)); got != coded {
		t.Fatalf("From(wrapped) = %v, want %v", got, coded)
	}
	if got := From(errors.New("plain")); got != nil {
		t.Fatalf("From(uncoded) = %v, want nil", got)
	}
	if got := From(nil); got != nil {
		t.Fatalf("From(nil) = %v, want nil", got)
	}
}

func TestErrorIsTheWireBody(t *testing.T) {
	encoded, err := json.Marshal(New(TooLarge, "too big"))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"code":"request_too_large","message":"too big"}` {
		t.Fatalf("wire body = %s", encoded)
	}
}
