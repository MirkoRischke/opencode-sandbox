package cli

import (
	"errors"
	"fmt"
	"io"

	"opencode-sandbox/internal/sandbox"

	"github.com/dustin/go-humanize"
)

// reportRemovals prints progress immediately and summarizes successes even when
// removal fails partway through. Output failures must not hide removal errors.
func reportRemovals(out io.Writer, kind string, remove func(sandbox.RemovalObserver) error) error {
	report := removalReport{out: out, kind: kind}
	err := remove(report.observe)
	if report.outputErr != nil {
		return errors.Join(err, report.outputErr)
	}
	if report.count == 0 && err == nil {
		_, outputErr := fmt.Fprintln(out, "Nothing to delete.")
		return outputErr
	}
	_, outputErr := fmt.Fprintln(out, report.summary())
	return errors.Join(err, outputErr)
}

type removalReport struct {
	out            io.Writer
	kind           string
	count, unknown int
	total          uint64
	outputErr      error
}

func (r *removalReport) observe(name string, size *uint64) {
	r.count++
	detail := ""
	if r.kind == "snapshot" {
		if size == nil {
			r.unknown++
			detail = " (size unknown)"
		} else {
			r.total += *size
			detail = " (" + humanize.IBytes(*size) + ")"
		}
	}
	if r.outputErr == nil {
		_, r.outputErr = fmt.Fprintf(r.out, "Deleted %s %s%s\n", r.kind, name, detail)
	}
}

func (r *removalReport) summary() string {
	plural := r.kind
	if r.count != 1 {
		plural += "s"
	}
	summary := fmt.Sprintf("Deleted %d %s", r.count, plural)
	if r.kind != "snapshot" || r.count == 0 {
		return summary
	}
	size := humanize.IBytes(r.total)
	if r.unknown == r.count {
		size = "unknown"
	} else if r.unknown > 0 {
		size += fmt.Sprintf(" + %d unknown", r.unknown)
	}
	return summary + " · total reported size: " + size
}
