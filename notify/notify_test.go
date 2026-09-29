package notify

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/kordloom/switchtender/sdk"
)

// intp returns a pointer to n for building runs.
func intp(n int) *int { return &n }

// timep returns a pointer to t for building runs.
func timep(t time.Time) *time.Time { return &t }

func TestFromRun(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)
	tests := []struct {
		Run      sdk.Run
		LinkBase string
		WantEv   Event
	}{{ // Test 0: A playbook run labels by playbook and links when a base is set.
		Run: sdk.Run{
			ID: "run_1", Playbook: "site.yml", Status: "succeeded",
			ExitCode: intp(0), StartedAt: timep(start), EndedAt: timep(end),
		},
		LinkBase: "https://yard.example.com/",
		WantEv: Event{
			ID: "run_1", Label: "site.yml", Tool: "ansible", Status: "succeeded",
			ExitCode: intp(0), Duration: 90 * time.Second,
			Link: "https://yard.example.com/ui/runs/run_1",
		},
	}, { // Test 1: A tool run with no playbook labels by tool and drops the command body.
		Run: sdk.Run{
			ID: "run_2", Tool: "bash", Command: "echo secret-script", Status: "failed",
			ExitCode: intp(7),
		},
		WantEv: Event{
			ID: "run_2", Label: "bash", Tool: "bash", Status: "failed", ExitCode: intp(7),
		},
	}, { // Test 2: A run that never started has no duration and no exit code.
		Run:    sdk.Run{ID: "run_3", Playbook: "p.yml", Status: "rejected"},
		WantEv: Event{ID: "run_3", Label: "p.yml", Tool: "ansible", Status: "rejected"},
	}, { // Test 3: An oversized label truncates to the cap.
		Run: sdk.Run{ID: "run_4", Playbook: strings.Repeat("x", 300), Status: "succeeded"},
		WantEv: Event{
			ID: "run_4", Label: strings.Repeat("x", labelCap), Tool: "ansible",
			Status: "succeeded",
		},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := FromRun(&test.Run, test.LinkBase)
			if diff := cmp.Diff(test.WantEv, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEventFacts(t *testing.T) {
	t.Parallel()
	event := Event{
		ID: "run_9", Label: "deploy.yml", Tool: "ansible", Status: "succeeded",
		ExitCode: intp(0), Duration: 3 * time.Second,
	}
	want := [][2]string{
		{"Tool", "ansible"}, {"Run", "run_9"}, {"Exit code", "0"}, {"Duration", "3s"},
	}
	if diff := cmp.Diff(want, event.Facts()); diff != "" {
		t.Errorf("facts mismatch (-want +got):\n%s", diff)
	}
	if got := event.Title(); got != "deploy.yml succeeded" {
		t.Errorf("Title() = %q, want deploy.yml succeeded", got)
	}
}
