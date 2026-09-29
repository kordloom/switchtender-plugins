package notify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/kordloom/switchtender/sdk"
)

// ticketCall is one request a fake ticketing system received.
type ticketCall struct {
	// Method is the request method.
	Method string
	// Path is the request path.
	Path string
	// Query is the decoded query string.
	Query url.Values
	// Auth is the Authorization header.
	Auth string
	// Body is the request body.
	Body []byte
}

// ticketServer returns a fake ticketing system that answers each request from reply and records
// it, and a function returning the calls it has received.
func ticketServer(t *testing.T, reply func(method, path string) (int, string)) (*httptest.Server,
	func() []ticketCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []ticketCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, ticketCall{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(),
			Auth: r.Header.Get("Authorization"), Body: body})
		mu.Unlock()
		status, payload := reply(r.Method, r.URL.Path)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []ticketCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]ticketCall(nil), calls...)
	}
}

// labeledRun is the channel tests' terminal run carrying change as its change label.
func labeledRun(change string) *sdk.Run {
	r := terminalRun()
	r.Labels = map[string]string{"change": change}
	return r
}

// basicAuth is the Authorization header basic auth sends for user and secret.
func basicAuth(user, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+secret))
}

// noteParts are what a ticket's note must say about terminalRun under the test link base.
var noteParts = []string{
	"SwitchTender run run_ch failed: bash, exit code 7, 2s.",
	"Run: https://yard.example/ui/runs/run_ch",
	"Evidence file: switchtender audit run run_ch",
}

// checkNote fails the test when note lacks a part it must carry or leaks the command body.
func checkNote(t *testing.T, note, change string) {
	t.Helper()
	parts := append([]string{"Every run on this change: https://yard.example/ui/runs?q=" +
		url.QueryEscape("label:change="+change)}, noteParts...)
	for _, want := range parts {
		if !strings.Contains(note, want) {
			t.Errorf("note does not say %q:\n%s", want, note)
		}
	}
	if strings.Contains(note, "topsecretscript") {
		t.Errorf("note leaked the command body:\n%s", note)
	}
}

// serviceNowFound answers a change lookup with one record and accepts the work note.
func serviceNowFound(method, _ string) (int, string) {
	if method == http.MethodGet {
		return http.StatusOK, `{"result":[{"sys_id":"0123456789abcdef0123456789abcdef"}]}`
	}
	return http.StatusOK, `{}`
}

// TestServiceNowRecordsTheRunOnItsChange pins the whole exchange: the change number is looked up
// exactly, the work note lands on the record the lookup returned, and every request carries the
// configured credentials.
func TestServiceNowRecordsTheRunOnItsChange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		User, Password, Token string
		WantAuth              string
	}{{ // Test 0: A user and password authenticate as basic auth.
		User: "svc", Password: "pw", WantAuth: basicAuth("svc", "pw"),
	}, { // Test 1: A token authenticates as a bearer, even beside a user.
		User: "svc", Password: "pw", Token: "tok", WantAuth: "Bearer tok",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv, calls := ticketServer(t, serviceNowFound)
			n := ServiceNow(srv.URL, test.User, test.Password, test.Token, "https://yard.example",
				srv.Client())
			if err := n.Notify(context.Background(), labeledRun("CHG0030001")); err != nil {
				t.Fatalf("Notify() error = %v", err)
			}
			got := calls()
			if len(got) != 2 {
				t.Fatalf("calls = %d, want a lookup and a work note", len(got))
			}
			lookup, note := got[0], got[1]
			if lookup.Method != http.MethodGet || lookup.Path != "/api/now/table/change_request" ||
				lookup.Query.Get("sysparm_query") != "number=CHG0030001" {
				t.Errorf("lookup = %s %s %v, want an exact number query", lookup.Method, lookup.Path,
					lookup.Query)
			}
			if note.Method != http.MethodPatch ||
				note.Path != "/api/now/table/change_request/0123456789abcdef0123456789abcdef" {
				t.Errorf("note = %s %s, want a patch of the record the lookup found", note.Method,
					note.Path)
			}
			var body map[string]string
			if err := json.Unmarshal(note.Body, &body); err != nil {
				t.Fatalf("note body %q is not JSON: %v", note.Body, err)
			}
			checkNote(t, body["work_notes"], "CHG0030001")
			for i, c := range got {
				if c.Auth != test.WantAuth {
					t.Errorf("call %d Authorization = %q, want %q", i, c.Auth, test.WantAuth)
				}
			}
		})
	}
}

// TestServiceNowLeavesOtherRunsAlone pins that a run is left alone unless its change label is a
// change number, exactly. A caret would start another condition in ServiceNow's encoded query and
// widen the lookup to a stranger's change, so near misses must send nothing at all.
func TestServiceNowLeavesOtherRunsAlone(t *testing.T) {
	t.Parallel()
	for testNum, change := range []string{
		"",                   // Test 0: No change label.
		"OPS-12",             // Test 1: A Jira key.
		"CHG1^ORactive=true", // Test 2: An encoded-query injection.
		"chg0030001",         // Test 3: Lowercase.
		"CHG",                // Test 4: No number.
		"CHG0030001 ",        // Test 5: Trailing space.
	} {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv, calls := ticketServer(t, serviceNowFound)
			r := labeledRun(change)
			if change == "" {
				r.Labels = nil
			}
			if err := ServiceNow(srv.URL, "svc", "pw", "", "", srv.Client()).Notify(
				context.Background(), r); err != nil {
				t.Fatalf("Notify() error = %v, want nil for a run that names no change", err)
			}
			if got := calls(); len(got) != 0 {
				t.Errorf("sent %d requests for change label %q, want none", len(got), change)
			}
		})
	}
}

// TestServiceNowReportsWhatWentWrong pins that every failed exchange surfaces as a delivery
// error the host logs, naming the change where it can.
func TestServiceNowReportsWhatWentWrong(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Lookup     string
		LookupCode int
		NoteCode   int
		WantText   string
	}{{ // Test 0: No record with that number.
		Lookup: `{"result":[]}`, LookupCode: 200, NoteCode: 200, WantText: "no change request CHG0030001",
	}, { // Test 1: Two records with that number.
		Lookup: `{"result":[{"sys_id":"0123456789abcdef0123456789abcdef"},` +
			`{"sys_id":"fedcba9876543210fedcba9876543210"}]}`,
		LookupCode: 200, NoteCode: 200, WantText: "more than one change request",
	}, { // Test 2: A record id that is not one is never put in a path.
		Lookup: `{"result":[{"sys_id":"../../sys_user/admin"}]}`, LookupCode: 200, NoteCode: 200,
		WantText: "unexpected record id",
	}, { // Test 3: The lookup is refused.
		Lookup: `{}`, LookupCode: 401, NoteCode: 200, WantText: "status 401",
	}, { // Test 4: The work note is refused.
		Lookup:     `{"result":[{"sys_id":"0123456789abcdef0123456789abcdef"}]}`,
		LookupCode: 200, NoteCode: 403, WantText: "status 403",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv, calls := ticketServer(t, func(method, _ string) (int, string) {
				if method == http.MethodGet {
					return test.LookupCode, test.Lookup
				}
				return test.NoteCode, `{}`
			})
			err := ServiceNow(srv.URL, "svc", "pw", "", "", srv.Client()).Notify(context.Background(),
				labeledRun("CHG0030001"))
			if !errors.Is(err, ErrDeliver) || !strings.Contains(err.Error(), test.WantText) {
				t.Errorf("Notify() error = %v, want ErrDeliver saying %q", err, test.WantText)
			}
			for _, c := range calls() {
				if strings.Contains(c.Path, "sys_user") {
					t.Errorf("a request reached %s", c.Path)
				}
			}
		})
	}
}

// TestJiraCommentsOnTheIssue pins the comment: posted to the issue the label names, carrying the
// note, with the configured credentials.
func TestJiraCommentsOnTheIssue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		User, Token string
		WantAuth    string
	}{{ // Test 0: A Jira Cloud user and API token authenticate as basic auth.
		User: "ops@example.com", Token: "tok", WantAuth: basicAuth("ops@example.com", "tok"),
	}, { // Test 1: A Data Center personal access token alone authenticates as a bearer.
		Token: "pat", WantAuth: "Bearer pat",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv, calls := ticketServer(t, func(string, string) (int, string) {
				return http.StatusCreated, `{}`
			})
			n := Jira(srv.URL, test.User, test.Token, "https://yard.example", srv.Client())
			if err := n.Notify(context.Background(), labeledRun("OPS-123")); err != nil {
				t.Fatalf("Notify() error = %v", err)
			}
			got := calls()
			if len(got) != 1 || got[0].Method != http.MethodPost ||
				got[0].Path != "/rest/api/2/issue/OPS-123/comment" {
				t.Fatalf("calls = %+v, want one comment on OPS-123", got)
			}
			var body map[string]string
			if err := json.Unmarshal(got[0].Body, &body); err != nil {
				t.Fatalf("comment body %q is not JSON: %v", got[0].Body, err)
			}
			checkNote(t, body["body"], "OPS-123")
			if got[0].Auth != test.WantAuth {
				t.Errorf("Authorization = %q, want %q", got[0].Auth, test.WantAuth)
			}
		})
	}
}

// TestJiraLeavesOtherRunsAlone pins that nothing but an issue key reaches the request path.
func TestJiraLeavesOtherRunsAlone(t *testing.T) {
	t.Parallel()
	for testNum, change := range []string{
		"",                 // Test 0: No change label.
		"CHG0030001",       // Test 1: A ServiceNow change number.
		"ops-123",          // Test 2: Lowercase.
		"OPS-0",            // Test 3: No issue is numbered zero.
		"OPS-1/../../user", // Test 4: A path traversal.
		"OPS-12?expand=1",  // Test 5: A smuggled query.
	} {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			srv, calls := ticketServer(t, func(string, string) (int, string) {
				return http.StatusCreated, `{}`
			})
			r := labeledRun(change)
			if change == "" {
				r.Labels = nil
			}
			n := Jira(srv.URL, "", "pat", "", srv.Client())
			if err := n.Notify(context.Background(), r); err != nil {
				t.Fatalf("Notify() error = %v, want nil for a run that names no issue", err)
			}
			if got := calls(); len(got) != 0 {
				t.Errorf("sent %d requests for change label %q, want none", len(got), change)
			}
		})
	}
}

// TestJiraReportsARejectedComment pins that a refused comment surfaces as a delivery error.
func TestJiraReportsARejectedComment(t *testing.T) {
	t.Parallel()
	srv, _ := ticketServer(t, func(string, string) (int, string) { return http.StatusNotFound, `{}` })
	err := Jira(srv.URL, "", "pat", "", srv.Client()).Notify(context.Background(), labeledRun("OPS-9"))
	if !errors.Is(err, ErrDeliver) {
		t.Errorf("Notify() error = %v, want ErrDeliver", err)
	}
}

// TestChangeNoteWithoutALinkBase pins that a note still says what ran and how to export its
// evidence when no link base is configured, and invents no links.
func TestChangeNoteWithoutALinkBase(t *testing.T) {
	t.Parallel()
	note := changeNote(labeledRun("OPS-5"), "OPS-5", "")
	if strings.Contains(note, "http") {
		t.Errorf("note carries a link with no link base configured:\n%s", note)
	}
	for _, want := range []string{noteParts[0], noteParts[2]} {
		if !strings.Contains(note, want) {
			t.Errorf("note does not say %q:\n%s", want, note)
		}
	}
}
