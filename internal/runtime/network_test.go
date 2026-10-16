package runtime_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	runtime "github.com/nnutter/moltisctl/internal/runtime"
)

const podmanNetworkJSON = `[{"name":"moltisctl-demo","id":"net123","driver":"bridge","labels":{"io.moltisctl.managed":"true","io.moltisctl.instance":"moltisctl-demo"}}]`

// Apple 0.9 NetworkState is a Swift associated-value enum. These labels are
// encoded inside the configuration (_0), not on the runtime subnet (_1).
const appleNetworkJSON = `[{"running":{"_0":{"id":"moltisctl-demo","mode":"nat","labels":{"io.moltisctl.managed":"true","io.moltisctl.instance":"moltisctl-demo"}},"_1":{"ipv4Subnet":"192.168.65.0/24","ipv4Gateway":"192.168.65.1"}}}]`

func networkFixture(platform string) string {
	if platform == "darwin" {
		return appleNetworkJSON
	}
	return podmanNetworkJSON
}

func networkAbsent(platform string) reply {
	operation := "ls"
	if platform == "darwin" {
		operation = "list"
	}
	return reply{args: []string{"network", operation, "--format", "json"}, output: "[]"}
}

func networkQuery(platform, json string) []reply {
	list := networkAbsent(platform)
	list.output = json
	return []reply{list, {args: []string{"network", "inspect", "moltisctl-demo"}, output: json}}
}

func networkCreateArgs() []string {
	return []string{"network", "create", "--label", "io.moltisctl.managed=true", "--label", "io.moltisctl.instance=moltisctl-demo", "moltisctl-demo"}
}

func networkMembers(platform, json string) reply {
	if platform == "darwin" {
		return reply{args: []string{"list", "--all", "--format", "json"}, output: json}
	}
	return reply{args: []string{"ps", "--all", "--filter", "network=moltisctl-demo", "--format", "json"}, output: json}
}

func TestRefuseForeignOrConflictingNetwork(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		for _, operation := range []string{"ensure-new", "ensure-existing", "start-existing", "remove-existing", "remove-orphan"} {
			for _, mismatch := range []string{"foreign", "instance"} {
				t.Run(platform+"/"+operation+"/"+mismatch, func(t *testing.T) {
					json := networkFixture(platform)
					if mismatch == "foreign" {
						json = strings.ReplaceAll(json, `"io.moltisctl.managed":"true"`, `"io.moltisctl.managed":"false"`)
					} else {
						json = strings.ReplaceAll(json, `"io.moltisctl.instance":"moltisctl-demo"`, `"io.moltisctl.instance":"moltisctl-other"`)
					}
					replies := []reply{absent(platform)}
					if strings.HasSuffix(operation, "existing") {
						replies = query(platform, fixture(platform))
					}
					replies = append(replies, networkQuery(platform, json)...)
					a, done := fakeCLI(t, platform, replies)
					var err error
					switch operation {
					case "ensure-new", "ensure-existing":
						err = a.Ensure(t.Context(), runtime.Spec{Name: "demo", Image: "localhost/moltisctl:latest", Home: "/tmp/home space", Port: 19000})
					case "start-existing":
						err = a.Start(t.Context(), "demo")
					default:
						err = a.Remove(t.Context(), "demo")
					}
					require.ErrorContains(t, err, "refuse")
					done()
				})
			}
		}
	}
}

func TestNetworkQueryFailuresAreNotAbsence(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		for _, scenario := range []string{"daemon", "malformed", "wrong-shape", "missing-identity", "inspect-failure", "inspect-identity"} {
			t.Run(platform+"/"+scenario, func(t *testing.T) {
				rows := []reply{networkAbsent(platform)}
				switch scenario {
				case "daemon":
					rows[0].output, rows[0].stderr, rows[0].exit = "", "daemon unavailable", 1
				case "malformed":
					rows[0].output = "[invalid"
				case "wrong-shape":
					rows[0].output = "null"
				case "missing-identity":
					rows[0].output = "[{}]"
				case "inspect-failure":
					rows = networkQuery(platform, networkFixture(platform))
					rows[1].output, rows[1].stderr, rows[1].exit = "", "inspect denied", 1
				case "inspect-identity":
					rows = networkQuery(platform, networkFixture(platform))
					rows[1].output = strings.ReplaceAll(rows[1].output, "moltisctl-demo", "moltisctl-other")
				}
				a, done := fakeCLI(t, platform, append([]reply{absent(platform)}, rows...))
				require.Error(t, a.Remove(t.Context(), "demo"))
				done()
			})
		}
	}
}

func TestRetryPartialNetworkRemoval(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		for _, result := range []string{"success", "concurrent", "still-present", "daemon"} {
			t.Run(platform+"/"+result, func(t *testing.T) {
				operation := "rm"
				if platform == "darwin" {
					operation = "delete"
				}
				replies := []reply{absent(platform)}
				replies = append(replies, networkQuery(platform, networkFixture(platform))...)
				replies = append(replies, networkMembers(platform, "[]"))
				mutation := reply{args: []string{"network", operation, "moltisctl-demo"}}
				if result != "success" {
					mutation.exit, mutation.stderr = 1, "network deletion failed"
				}
				replies = append(replies, mutation)
				switch result {
				case "concurrent":
					replies = append(replies, networkAbsent(platform))
				case "still-present":
					replies = append(replies, networkQuery(platform, networkFixture(platform))...)
				case "daemon":
					failed := networkAbsent(platform)
					failed.exit, failed.stderr = 1, "daemon unavailable"
					replies = append(replies, failed)
				}
				a, done := fakeCLI(t, platform, replies)
				err := a.Remove(t.Context(), "demo")
				switch result {
				case "success", "concurrent":
					require.NoError(t, err)
				default:
					require.ErrorContains(t, err, "network deletion failed")
				}
				done()
			})
		}
	}
}

func TestRefuseNonemptyNetwork(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			replies := []reply{absent(platform)}
			replies = append(replies, networkQuery(platform, networkFixture(platform))...)
			members := `[{"Id":"other","Names":["foreign"]}]`
			if platform == "darwin" {
				// A stopped container still reserves its configured network.
				members = fixture(platform)
			}
			replies = append(replies, networkMembers(platform, members))
			a, done := fakeCLI(t, platform, replies)
			require.ErrorContains(t, a.Remove(t.Context(), "demo"), "nonempty")
			done()
		})
	}
}

func TestDistinctInstanceNetworks(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			var replies []reply
			for _, name := range []string{"demo", "second"} {
				commands := []reply{absent(platform), networkAbsent(platform), {args: networkCreateArgs()}}
				commands = append(commands, networkQuery(platform, networkFixture(platform))...)
				commands = append(commands, reply{args: createArgs(platform)})
				commands = append(commands, query(platform, fixture(platform))...)
				for _, command := range commands {
					for i, arg := range command.args {
						command.args[i] = strings.ReplaceAll(arg, "moltisctl-demo", "moltisctl-"+name)
					}
					command.output = strings.ReplaceAll(command.output, "moltisctl-demo", "moltisctl-"+name)
					replies = append(replies, command)
				}
			}
			a, done := fakeCLI(t, platform, replies)
			for _, name := range []string{"demo", "second"} {
				require.NoError(t, a.Ensure(t.Context(), runtime.Spec{Name: name, Image: "localhost/moltisctl:latest", Home: "/tmp/home space", Port: 19000}))
			}
			done()
		})
	}
}

func TestCreatedContainerMustMatchNetwork(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			replies := []reply{absent(platform), networkAbsent(platform), {args: networkCreateArgs()}}
			replies = append(replies, networkQuery(platform, networkFixture(platform))...)
			replies = append(replies, reply{args: createArgs(platform)})
			replies = append(replies, query(platform, unsafeFixture(platform, "network"))...)
			a, done := fakeCLI(t, platform, replies)
			require.ErrorContains(t, a.Ensure(t.Context(), runtime.Spec{Name: "demo", Image: "localhost/moltisctl:latest", Home: "/tmp/home space", Port: 19000}), "verification")
			done()
		})
	}
}

func TestAppleNetworkFormats(t *testing.T) {
	for _, json := range []string{
		`[{"created":{"_0":{"id":"moltisctl-demo","mode":"nat","labels":{"io.moltisctl.managed":"true","io.moltisctl.instance":"moltisctl-demo"}}}}]`,
		`[{"id":"moltisctl-demo","configuration":{"name":"moltisctl-demo","mode":"nat","labels":{"io.moltisctl.managed":"true","io.moltisctl.instance":"moltisctl-demo"}},"status":{"ipv4Subnet":"192.168.65.0/24","ipv4Gateway":"192.168.65.1"}}]`,
	} {
		t.Run(json, func(t *testing.T) {
			replies := query("darwin", appleJSON)
			replies = append(replies, networkQuery("darwin", json)...)
			a, done := fakeCLI(t, "darwin", replies)
			require.NoError(t, a.Ensure(t.Context(), runtime.Spec{Name: "demo", Image: "localhost/moltisctl:latest", Home: "/tmp/home space", Port: 19000}))
			done()
		})
	}
}

func TestAppleRunningNetworkAttachments(t *testing.T) {
	for _, attachment := range []string{`[]`, `[{"network":"default"}]`, `[{"network":"moltisctl-demo"},{"network":"default"}]`} {
		t.Run(attachment, func(t *testing.T) {
			json := strings.ReplaceAll(appleJSON, `"status":"stopped"`, `"status":"running"`)
			json = strings.ReplaceAll(json, `"networks":[]`, `"networks":`+attachment)
			a, done := fakeCLI(t, "darwin", query("darwin", json))
			require.ErrorContains(t, a.Start(t.Context(), "demo"), "expected only network")
			done()
		})
	}
}

func TestNetworkCreationRace(t *testing.T) {
	for _, platform := range []string{"linux", "darwin"} {
		for _, owned := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/owned=%t", platform, owned), func(t *testing.T) {
				replies := []reply{absent(platform), networkAbsent(platform), {args: networkCreateArgs(), exit: 1, stderr: "already exists"}}
				json := networkFixture(platform)
				if !owned {
					json = strings.ReplaceAll(json, `"io.moltisctl.managed":"true"`, `"io.moltisctl.managed":"false"`)
				}
				replies = append(replies, networkQuery(platform, json)...)
				if owned {
					replies = append(replies, reply{args: createArgs(platform)})
					replies = append(replies, query(platform, fixture(platform))...)
				}
				a, done := fakeCLI(t, platform, replies)
				err := a.Ensure(t.Context(), runtime.Spec{Name: "demo", Image: "localhost/moltisctl:latest", Home: "/tmp/home space", Port: 19000})
				if owned {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				done()
			})
		}
	}
}
