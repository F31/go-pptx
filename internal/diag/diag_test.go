package diag

import "testing"

func TestSeverityString(t *testing.T) {
	for _, tc := range []struct {
		s    Severity
		want string
	}{
		{SeverityInfo, "info"},
		{SeverityWarning, "warning"},
		{SeverityError, "error"},
		{Severity(99), "unknown"},
	} {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("Severity(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestValidationReportHasErrors(t *testing.T) {
	empty := ValidationReport{Mode: "structural"}
	if empty.HasErrors() {
		t.Fatal("empty report should not have errors")
	}
	warnOnly := ValidationReport{Diagnostics: []Diagnostic{{Severity: SeverityWarning}, {Severity: SeverityInfo}}}
	if warnOnly.HasErrors() {
		t.Fatal("warning/info only should not report errors")
	}
	withErr := ValidationReport{Diagnostics: []Diagnostic{{Severity: SeverityInfo}, {Severity: SeverityError}}}
	if !withErr.HasErrors() {
		t.Fatal("report containing an error should report errors")
	}
}
