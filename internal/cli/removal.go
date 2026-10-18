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
	var count, unknown int
	var total uint64
	var outputErr error
	err := remove(func(name string, size *uint64) {
		count++
		detail := ""
		if kind == "snapshot" {
			if size == nil {
				unknown++
				detail = " (size unknown)"
			} else {
				total += *size
				detail = " (" + humanize.IBytes(*size) + ")"
			}
		}
		if outputErr == nil {
			_, outputErr = fmt.Fprintf(out, "Deleted %s %s%s\n", kind, name, detail)
		}
	})
	if outputErr != nil {
		return errors.Join(err, outputErr)
	}
	if count == 0 && err == nil {
		_, outputErr = fmt.Fprintln(out, "Nothing to delete.")
		return outputErr
	}
	plural := kind
	if count != 1 {
		plural += "s"
	}
	summary := fmt.Sprintf("Deleted %d %s", count, plural)
	if kind == "snapshot" && count > 0 {
		size := humanize.IBytes(total)
		if unknown == count {
			size = "unknown"
		} else if unknown > 0 {
			size += fmt.Sprintf(" + %d unknown", unknown)
		}
		summary += " · total reported size: " + size
	}
	_, outputErr = fmt.Fprintln(out, summary)
	return errors.Join(err, outputErr)
}
