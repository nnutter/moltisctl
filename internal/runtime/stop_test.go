package runtime_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStopRequiresObservedStoppedStateAndPreservesResources(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		for _, scenario := range []string{"running", "already stopped", "missing", "still running", "query failed", "disappeared"} {
			t.Run(platform+"/"+scenario, func(t *testing.T) {
				id := "abc123"
				if platform == "darwin" {
					id = "moltisctl-demo"
				}
				json := fixture(platform)
				var replies []reply
				if scenario == "missing" {
					replies = []reply{absent(platform)}
				} else {
					initial := json
					if scenario != "already stopped" {
						initial = running(platform, json)
					}
					replies = query(platform, initial)
					replies = append(replies, networkQuery(platform, networkFixture(platform))...)
					if scenario != "already stopped" {
						replies = append(replies, reply{args: []string{"stop", id}})
						switch scenario {
						case "still running":
							replies = append(replies, query(platform, running(platform, json))...)
						case "query failed":
							failed := absent(platform)
							failed.exit, failed.stderr = 1, "daemon unavailable"
							replies = append(replies, failed)
						case "disappeared":
							replies = append(replies, absent(platform))
						default:
							replies = append(replies, query(platform, json)...)
						}
					}
				}
				a, done := fakeCLI(t, platform, replies)
				err := a.Stop(t.Context(), "demo")
				if scenario == "running" || scenario == "already stopped" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				done() // No remove, network deletion, or recreation is permitted.
			})
		}
	}
}
