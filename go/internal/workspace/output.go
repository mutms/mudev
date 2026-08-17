package workspace

import (
	"fmt"
	"io"
	"os"
)

// output is where mudev's own progress lines go.
//
// git writes its transfer progress straight to the terminal, so mudev's job is
// to say which repository and which remote that progress belongs to: a fetch
// of a million objects looks identical whether it is coming from the mirror
// next door or from the other side of the internet.
type output struct {
	w io.Writer
}

func newOutput(w io.Writer) output {
	if w == nil {
		w = os.Stdout
	}

	return output{w: w}
}

// printf writes a top-level line: one checkout, one decision.
func (o output) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(o.w, format+"\n", args...)
}

// stepf writes an indented line underneath, for the individual git operations
// a checkout is made of.
func (o output) stepf(format string, args ...any) {
	_, _ = fmt.Fprintf(o.w, "  "+format+"\n", args...)
}

// warnf reports something the user should know about that does not stop the
// run.
func (o output) warnf(format string, args ...any) {
	_, _ = fmt.Fprintf(o.w, "warning: "+format+"\n", args...)
}
