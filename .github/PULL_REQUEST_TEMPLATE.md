## What this changes, and why

<!-- The diff says what. Say why. -->

## Checks

- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...` is clean
- [ ] `go test -race ./...` passes

## If this touches scoring

<!-- Delete this section if it does not. -->

- [ ] Said what it does to a test that was previously `flaky`, `suspect`,
      `stable` and `consistently-failing`
- [ ] `./scripts/reproduce-validation.sh` still flips the three ConduitIO tests
- [ ] An always-failing test still scores 0.00 and reports as
      `consistently-failing`
- [ ] If a default moved, argued against the reasoning in VALIDATION.md rather
      than from intuition
