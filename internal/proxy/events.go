package proxy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
)

type upEventTracker struct {
	totalRegistered int
	registeredURNs  map[string]struct{}

	createSucceeded int
	createFailed    int

	diagnostics   map[string][]string
	generalErrors []string
	failures      []string
}

func newUpEventTracker() *upEventTracker {
	return &upEventTracker{
		registeredURNs: make(map[string]struct{}),
		diagnostics:    make(map[string][]string),
	}
}

func (t *upEventTracker) consume(events <-chan events.EngineEvent) {
	for evt := range events {
		t.handle(evt)
	}
}

func (t *upEventTracker) handle(event events.EngineEvent) {
	if event.Error != nil {
		t.recordDiagnostic("", event.Error.Error())
		return
	}
	if pre := event.ResourcePreEvent; pre != nil {
		urn := pre.Metadata.URN
		if urn == "" {
			t.totalRegistered++
		} else if _, ok := t.registeredURNs[urn]; !ok {
			t.registeredURNs[urn] = struct{}{}
			t.totalRegistered++
		}
		return
	}
	if diag := event.DiagnosticEvent; diag != nil {
		if strings.Contains(strings.ToLower(diag.Severity), "error") {
			t.recordDiagnostic(diag.URN, diag.Message)
		}
		return
	}
	if failed := event.ResOpFailedEvent; failed != nil {
		if isCreateLike(failed.Metadata.Op) {
			t.createFailed++
			urn := failed.Metadata.URN
			msg, urnSpecific := t.takeDiagnostic(urn)
			if urnSpecific && urn != "" && msg != "" {
				t.failures = append(t.failures, fmt.Sprintf("%s: %s", urn, msg))
			} else if msg != "" {
				t.failures = append(t.failures, msg)
			} else if urn != "" {
				t.failures = append(t.failures, fmt.Sprintf("%s: operation failed", urn))
			} else {
				t.failures = append(t.failures, "Resource operation failed")
			}
		}
		return
	}
	if out := event.ResOutputsEvent; out != nil && isCreateLike(out.Metadata.Op) {
		t.createSucceeded++
		return
	}
}

func (t *upEventTracker) recordDiagnostic(urn, message string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}
	if urn == "" {
		t.generalErrors = append(t.generalErrors, msg)
		return
	}
	t.diagnostics[urn] = append(t.diagnostics[urn], msg)
}

func (t *upEventTracker) takeDiagnostic(urn string) (string, bool) {
	if urn != "" {
		if msgs, ok := t.diagnostics[urn]; ok && len(msgs) > 0 {
			delete(t.diagnostics, urn)
			return strings.Join(msgs, "\n"), true
		}
	}
	if len(t.generalErrors) > 0 {
		msg := strings.Join(t.generalErrors, "\n")
		t.generalErrors = nil
		return msg, false
	}
	return "", false
}

func (t *upEventTracker) failureSummary() string {
	var parts []string
	parts = append(parts, t.failures...)

	if len(t.diagnostics) > 0 {
		urns := make([]string, 0, len(t.diagnostics))
		for urn := range t.diagnostics {
			if urn == "" {
				continue
			}
			urns = append(urns, urn)
		}
		sort.Strings(urns)
		for _, urn := range urns {
			msgs := t.diagnostics[urn]
			if len(msgs) == 0 {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s: %s", urn, strings.Join(msgs, "\n")))
		}
	}

	if len(t.generalErrors) > 0 {
		parts = append(parts, strings.Join(t.generalErrors, "\n"))
	}

	return strings.Join(parts, "\n\n")
}

func (t *upEventTracker) created() int {
	return t.createSucceeded
}

func (t *upEventTracker) failedCreates() int {
	return t.createFailed
}

func (t *upEventTracker) totalResources() int {
	return t.totalRegistered
}

func isCreateLike(op apitype.OpType) bool {
	switch op {
	case apitype.OpCreate, apitype.OpCreateReplacement, apitype.OpImport, apitype.OpImportReplacement:
		return true
	default:
		return false
	}
}
