package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/kordloom/switchtender/sdk"
)

// serviceNowChange matches a ServiceNow change request number. The match is exact on purpose:
// the number goes into an encoded query, where a caret starts another condition, so a label such
// as "CHG1^ORactive=true" is left alone rather than let widen the lookup to someone else's change.
var serviceNowChange = regexp.MustCompile(`^CHG[0-9]{1,20}$`)

// serviceNowSysID matches the record id ServiceNow returns, checked before it is put in a path.
var serviceNowSysID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ServiceNow returns a notifier that records each terminal run as a work note on the ServiceNow
// change request its change label names. A run without a change label, or whose label is not a
// change number, is left alone, so the channel only speaks where a team tied a run to a change.
// A token authenticates as a bearer; otherwise the user and password authenticate as basic auth.
func ServiceNow(instance, user, password, token, linkBase string, client *http.Client,
) sdk.Notifier {
	base := strings.TrimRight(instance, "/")
	auth := ticketAuth(user, password)
	if token != "" {
		auth = ticketAuth("", token)
	}
	return sdk.NotifierFunc(func(ctx context.Context, r *sdk.Run) error {
		change := r.Labels[changeLabel]
		if !serviceNowChange.MatchString(change) {
			return nil
		}
		query := url.Values{
			"sysparm_query":  {"number=" + change},
			"sysparm_fields": {"sys_id"},
			"sysparm_limit":  {"2"},
		}
		reply, err := ticketRequest(ctx, client, http.MethodGet,
			base+"/api/now/table/change_request?"+query.Encode(), nil, auth)
		if err != nil {
			return err
		}
		var found struct {
			// Result holds the matching change requests.
			Result []struct {
				// SysID is the record id.
				SysID string `json:"sys_id"`
			} `json:"result"`
		}
		if err := json.Unmarshal(reply, &found); err != nil {
			return fmt.Errorf("%w: read the ServiceNow lookup for %s: %w", ErrDeliver, change, err)
		}
		switch {
		case len(found.Result) == 0:
			return fmt.Errorf("%w: ServiceNow has no change request %s", ErrDeliver, change)
		case len(found.Result) > 1:
			return fmt.Errorf("%w: ServiceNow has more than one change request numbered %s",
				ErrDeliver, change)
		case !serviceNowSysID.MatchString(found.Result[0].SysID):
			return fmt.Errorf("%w: ServiceNow returned an unexpected record id for %s", ErrDeliver, change)
		}
		_, err = ticketRequest(ctx, client, http.MethodPatch,
			base+"/api/now/table/change_request/"+found.Result[0].SysID,
			map[string]string{"work_notes": changeNote(r, change, linkBase)}, auth)
		return err
	})
}
