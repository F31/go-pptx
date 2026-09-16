package errs

import (
	"errors"
	"testing"
)

func TestOperationErrorError(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    *OperationError
		want string
	}{
		{"empty", &OperationError{}, "pptx: operation failed"},
		{"op+err", &OperationError{Op: "Open", Err: ErrNotFound}, "pptx: Open: pptx: not found"},
		{"message+err", &OperationError{Op: "Open", Message: "ctx", Err: ErrNotFound}, "pptx: Open: ctx: pptx: not found"},
		{"message only", &OperationError{Op: "Open", Message: "ctx"}, "pptx: Open: ctx"},
		{"no op", &OperationError{Err: ErrClosed}, "pptx: document closed"},
		{"no op message", &OperationError{Message: "only"}, "only"},
	} {
		if got := tc.e.Error(); got != tc.want {
			t.Errorf("%s: Error() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestOperationErrorUnwrap(t *testing.T) {
	err := &OperationError{Op: "Open", Err: ErrNotFound}
	if !errors.Is(err, ErrNotFound) {
		t.Fatal("errors.Is should find sentinel via Unwrap")
	}
	var oe *OperationError
	if !errors.As(err, &oe) || oe.Op != "Open" {
		t.Fatalf("errors.As failed: %+v", oe)
	}
}

func TestAnnotate(t *testing.T) {
	if Annotate(nil, "Open") != nil {
		t.Fatal("Annotate(nil) should return nil")
	}
	got := Annotate(ErrClosed, "Open")
	if !errors.Is(got, ErrClosed) {
		t.Fatalf("Annotate lost sentinel: %v", got)
	}
	var oe *OperationError
	if !errors.As(got, &oe) || oe.Op != "Open" {
		t.Fatalf("Annotate op = %+v", oe)
	}
}
