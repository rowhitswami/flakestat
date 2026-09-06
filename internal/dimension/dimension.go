// Package dimension records the context an observation was made in, so that
// later analysis can ask whether failures cluster on one platform, runtime or
// CI worker.
//
// Two rules keep the data honest, because a correlation drawn over wrong
// metadata is worse than no correlation at all:
//
//   - Nothing is inferred. Values are captured from a whitelist of known CI
//     variables, from JUnit properties flakestat explicitly recognizes, or
//     from the user. Arbitrary process environment is never scraped, and the
//     runtime under test is never guessed at.
//
//   - Host details are only recorded when flakestat actually launched the
//     tests. Ingesting a report produced last week on a Windows runner must
//     not stamp it with the laptop doing the ingesting.
//
// Values are opaque: this package does not interpret "3.13" or "windows-latest".
package dimension

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Canonical keys owned by flakestat. Anything else is a user dimension.
const (
	OS   = "os"
	Arch = "arch"

	CIProvider = "ci.provider"
	CIRunID    = "ci.run_id"
	CIJobID    = "ci.job_id"
	CIWorker   = "ci.worker"
	CIShard    = "ci.shard"

	RuntimeName    = "runtime.name"
	RuntimeVersion = "runtime.version"
)

// Set is the context attached to an observation.
type Set map[string]string

// keyPattern is deliberately conservative: dimension keys become grouping
// columns and appear in reports, so odd characters would leak into output.
var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateKey rejects malformed keys early, where the message can name the
// offending input.
func ValidateKey(k string) error {
	if k == "" {
		return fmt.Errorf("key is empty")
	}
	if !keyPattern.MatchString(k) {
		return fmt.Errorf("key %q may only contain letters, digits, dot, dash or underscore, and must start with a letter or digit", k)
	}
	return nil
}

// Parse reads repeated key=value flags.
func Parse(pairs []string) (Set, error) {
	out := Set{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("invalid dimension %q: expected key=value", p)
		}
		k = strings.TrimSpace(k)
		// Naming the original pair matters: "key is empty" alone leaves the
		// user hunting through several --dimension flags for the bad one.
		if err := ValidateKey(k); err != nil {
			return nil, fmt.Errorf("invalid dimension %q: %w", p, err)
		}
		out[k] = strings.TrimSpace(v)
	}
	return out, nil
}

// Merge layers sets so that later ones win. Callers layer in precedence order:
// automatic detection, then recognized JUnit properties, then explicit flags,
// so the user always has the final say.
func Merge(layers ...Set) Set {
	out := Set{}
	for _, l := range layers {
		for k, v := range l {
			if v == "" {
				continue
			}
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Keys returns the dimension names in a stable order.
func (s Set) Keys() []string {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Host reports the machine flakestat is running on.
//
// Only valid when flakestat launched the tests itself. Applying it to an
// ingested report would invent execution context that never existed.
func Host() Set {
	return Set{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
}

// provider describes how to recognize one CI system and which of its variables
// are safe and useful to read.
type provider struct {
	name    string
	detect  string // env var whose presence identifies the provider
	runID   string
	jobID   string
	worker  string
	shard   string
	require string // optional exact value the detect var must have
}

// Whitelisted providers. Only these variables are ever read: scraping the
// process environment would eventually capture tokens and secrets.
var providers = []provider{
	{name: "github", detect: "GITHUB_ACTIONS", require: "true",
		runID: "GITHUB_RUN_ID", jobID: "GITHUB_JOB"},
	{name: "gitlab", detect: "GITLAB_CI", require: "true",
		runID: "CI_PIPELINE_ID", jobID: "CI_JOB_ID", shard: "CI_NODE_INDEX"},
	{name: "circleci", detect: "CIRCLECI", require: "true",
		runID: "CIRCLE_WORKFLOW_ID", jobID: "CIRCLE_JOB", shard: "CIRCLE_NODE_INDEX"},
	{name: "buildkite", detect: "BUILDKITE", require: "true",
		runID: "BUILDKITE_BUILD_ID", jobID: "BUILDKITE_JOB_ID", shard: "BUILDKITE_PARALLEL_JOB"},
	{name: "jenkins", detect: "JENKINS_URL",
		runID: "BUILD_ID", jobID: "JOB_NAME"},
	{name: "azure", detect: "TF_BUILD", require: "True",
		runID: "BUILD_BUILDID", jobID: "AGENT_JOBNAME"},
}

// DetectCI reports the recognized CI context, or nil when not running in one.
func DetectCI() Set {
	for _, p := range providers {
		val, present := os.LookupEnv(p.detect)
		if !present || val == "" {
			continue
		}
		if p.require != "" && !strings.EqualFold(val, p.require) {
			continue
		}

		s := Set{CIProvider: p.name}
		for key, env := range map[string]string{
			CIRunID: p.runID, CIJobID: p.jobID, CIWorker: p.worker, CIShard: p.shard,
		} {
			if env == "" {
				continue
			}
			if v := strings.TrimSpace(os.Getenv(env)); v != "" {
				s[key] = v
			}
		}
		return s
	}
	return nil
}

// InCI reports whether a recognized CI provider was detected. Ingesting inside
// CI means the ingesting machine did run the tests, which is what makes it
// safe to record host details.
func InCI() bool { return DetectCI() != nil }

// recognizedProperties maps JUnit <property> names flakestat understands onto
// canonical dimensions.
//
// Only these are read. Serializing every property would eventually ingest
// secrets, absolute paths, JVM arguments and high-cardinality build ids, none
// of which group usefully and some of which should never be stored.
var recognizedProperties = map[string]string{
	"os.name": OS,
	"os.arch": Arch,
}

// runtimeProperties map a property name onto the runtime it identifies, so a
// version can be attributed without guessing what produced it.
var runtimeProperties = map[string]string{
	"java.version":   "java",
	"go.version":     "go",
	"python.version": "python",
	"node.version":   "node",
	"ruby.version":   "ruby",
}

// FromProperties extracts dimensions from JUnit properties flakestat
// recognizes, ignoring everything else.
func FromProperties(props map[string]string) Set {
	out := Set{}
	for name, value := range props {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if key, ok := recognizedProperties[strings.ToLower(name)]; ok {
			out[key] = value
		}
		if rt, ok := runtimeProperties[strings.ToLower(name)]; ok {
			out[RuntimeName] = rt
			out[RuntimeVersion] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
