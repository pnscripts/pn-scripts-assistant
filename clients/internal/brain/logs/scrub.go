package logs

import (
	"context"
	"log/slog"

	"pn-scripts-assistant/internal/brain/redact"
)

/*
 * scrubbed is a log handler that takes secrets out on the way to the file.
 *
 * Every other place that keeps text — evidence, the conversation — passes
 * through redact first. The log did not, and the log is the one that is read
 * by somebody else: it is what a person sends when they ask for help, and it
 * is the file a support request attaches without reading. A key printed by an
 * approved command, or a URL with a token in its query, reached it exactly as
 * it was.
 *
 * Wrapping the handler rather than asking every caller to remember: there are
 * hundreds of log lines and the one that forgets is the one that matters.
 * Both the message and the values are scrubbed, because either can carry it —
 * `slog.Info("running", "command", "curl -H 'Authorization: Bearer …'")` puts
 * it in the value, and an error wrapped three deep puts it in the message.
 */
type scrubbed struct{ next slog.Handler }

func (s scrubbed) Enabled(ctx context.Context, level slog.Level) bool {
	return s.next.Enabled(ctx, level)
}

func (s scrubbed) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, redact.Text(record.Message), record.PC)

	record.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(scrubAttr(a))

		return true
	})

	return s.next.Handle(ctx, clean)
}

func (s scrubbed) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(attrs))

	for _, a := range attrs {
		clean = append(clean, scrubAttr(a))
	}

	return scrubbed{next: s.next.WithAttrs(clean)}
}

func (s scrubbed) WithGroup(name string) slog.Handler {
	return scrubbed{next: s.next.WithGroup(name)}
}

/*
 * scrubAttr cleans one value.
 *
 * Strings and errors are the ones that carry secrets; numbers and times
 * cannot, and running the regexes over them would cost time for nothing. A
 * group is walked because an attribute can hold attributes.
 */
func scrubAttr(a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, redact.Text(a.Value.String()))

	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok && err != nil {
			return slog.String(a.Key, redact.Text(err.Error()))
		}

		// Anything else printed by its own String method can carry one too.
		if text, ok := a.Value.Any().(interface{ String() string }); ok {
			return slog.String(a.Key, redact.Text(text.String()))
		}

		return a

	case slog.KindGroup:
		inner := a.Value.Group()
		clean := make([]slog.Attr, 0, len(inner))

		for _, each := range inner {
			clean = append(clean, scrubAttr(each))
		}

		return slog.Attr{Key: a.Key, Value: slog.GroupValue(clean...)}

	default:
		return a
	}
}
