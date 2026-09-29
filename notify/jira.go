package notify

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/kordloom/switchtender/sdk"
)

// jiraIssue matches a Jira issue key. The match is exact because the key goes into a request path,
// so nothing but a project key, a hyphen, and a number may reach it.
var jiraIssue = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,254}-[1-9][0-9]{0,18}$`)

// Jira returns a notifier that records each terminal run as a comment on the Jira issue its
// change label names. A run without a change label, or whose label is not an issue key, is left
// alone. A user authenticates with the token as basic auth, as Jira Cloud's API tokens do; a token
// alone authenticates as a bearer, as Jira Data Center's personal access tokens do.
func Jira(base, user, token, linkBase string, client *http.Client) sdk.Notifier {
	root := strings.TrimRight(base, "/")
	auth := ticketAuth(user, token)
	return sdk.NotifierFunc(func(ctx context.Context, r *sdk.Run) error {
		key := r.Labels[changeLabel]
		if !jiraIssue.MatchString(key) {
			return nil
		}
		_, err := ticketRequest(ctx, client, http.MethodPost, root+"/rest/api/2/issue/"+key+"/comment",
			map[string]string{"body": changeNote(r, key, linkBase)}, auth)
		return err
	})
}
