package store

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// Execution identity answers a question the observation log cannot otherwise
// answer: is this line a new execution, or the same execution described a
// second time?
//
// It matters because a duplicate is not neutral. A copy carries the timestamp
// of the execution it copies, so it sorts adjacent to its original, and a
// duplicate always agrees with itself: every copied observation inserts an
// artificial agreement into the series.
//
// The effect is measured rather than assumed. A test failing 4 times in 12,
// ingested a second time, scored 0.64 -> 0.30 and its confidence bound fell
// from 0.510 to 0.230 -- while the recorded evidence appeared to double. The
// harm runs towards false negatives: duplication makes a flaky test look
// stable and lends that wrong answer more apparent support.
//
// The identity is derived entirely from the evidence. Nothing about when, how
// often, or on which machine ingestion happened may enter it, or re-ingesting
// would produce a different key and defeat the whole point.

// ExecutionScope is where an execution happened, as far as anything can tell.
type ExecutionScope struct {
	// Context is the recorded dimensions of the execution: platform, runtime,
	// CI provider, run, job, attempt, shard and any user-supplied axis.
	//
	// The whole set is used rather than a chosen few, because a provider's
	// idea of a job may be coarser than an execution. GitHub reports the same
	// GITHUB_JOB for every leg of a matrix, so three platforms ingesting under
	// one run and one job are distinguishable only by what they recorded about
	// themselves. Anything that changes the recorded context makes this a
	// different execution.
	//
	// The attempt inside it is what keeps a legitimate retry distinguishable
	// from a duplicate: a re-run really did execute the tests again.
	Context map[string]string

	// Report is the cleaned path the results were read from and Digest a hash
	// of its bytes.
	//
	// Both are needed. The digest alone would merge two shards that ran
	// different tests but happened to emit identical XML; the path alone would
	// merge a file with the file that later replaced it. Together they say
	// "this exact document, from this exact place".
	Report string
	Digest string
}

// canonical renders the context in a fixed order so that map iteration cannot
// change a key.
func (s ExecutionScope) canonical() string {
	keys := make([]string, 0, len(s.Context))
	for k, v := range s.Context {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(s.Context[k])
		b.WriteByte('\n')
	}
	return b.String()
}

// Digest hashes report bytes for use as part of an execution scope.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Empty reports whether the scope identifies nothing, in which case no key can
// be formed and the observation must be kept unconditionally.
func (s ExecutionScope) Empty() bool {
	return s.Report == "" && s.Digest == "" && s.canonical() == ""
}

// ExecutionKey identifies the ordinal-th execution of testID within scope.
//
// The ordinal is essential rather than defensive. Running a suite with
// -count=12 puts twelve entries for one test in a single document, and they
// are twelve real executions; without an ordinal they would collapse into one
// and eleven results would silently vanish.
func ExecutionKey(scope ExecutionScope, testID string, ordinal int) string {
	if scope.Empty() {
		return ""
	}

	// Length-prefixed so that no combination of field values can be rearranged
	// into another valid key. Without it a report path ending in one character
	// and a digest beginning with another would be interchangeable.
	var b strings.Builder
	for _, part := range []string{
		scope.canonical(), scope.Report, scope.Digest, testID, strconv.Itoa(ordinal),
	} {
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}

	sum := sha256.Sum256([]byte(b.String()))
	// Half the digest: 64 bits over a log of at most millions of executions
	// leaves collision probability negligible, and the log stays readable.
	return hex.EncodeToString(sum[:8])
}

// Dedup drops observations whose execution has already been seen, keeping the
// first of each.
//
// This runs on read rather than only on write because the log is designed to
// be merged with cat. Concatenating the artifacts of several CI jobs never
// passes through Append, so a duplicate introduced that way would otherwise
// never be caught.
//
// Observations with no key predate execution identity or came from a source
// that cannot form one. They are always kept: silently discarding evidence
// that merely might be duplicated would be the worse error.
func Dedup(obs []Observation) []Observation {
	seen := make(map[string]struct{}, len(obs))
	out := obs[:0:0]
	for _, o := range obs {
		if o.ExecKey != "" {
			if _, dup := seen[o.ExecKey]; dup {
				continue
			}
			seen[o.ExecKey] = struct{}{}
		}
		out = append(out, o)
	}
	return out
}
