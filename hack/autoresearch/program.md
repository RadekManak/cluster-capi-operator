# Autoresearch Strategic Directive

## Goal
Your objective is to optimize the performance of the `machinesync` controller and its corresponding tests, significantly reducing the total test execution time while maintaining functional correctness.

You are acting as an autonomous software researcher. This directory contains the scaffolding to enable a continuous optimization loop (the "Autoresearch paradigm") for the OpenShift Cluster CAPI Operator.

## The Rules of the Game
1. **The Target Files (Mutable)**: You may explore and modify the controller logic and tests inside `pkg/controllers/machinesync/`. You have unrestricted freedom to alter Go code, refactor logic, or optimize how tests run, provided you adhere to Go language standards and the OpenShift Operator patterns.
2. **The Evaluator (Immutable)**: You must **NOT** modify `hack/autoresearch/evaluator.py`. This script represents the absolute trust boundary. It defines how success is measured.
3. **The Metric**: Your primary optimization metric is the execution `Duration` reported by the evaluator script.
4. **The Constraint**: The test suite must pass (`Status: SUCCESS`). Any change that reduces execution time but causes a test failure is an invalid iteration.

## The Execution Cycle (The Ratchet Loop)
As an autonomous agent, you should execute the following sequence:

1. **Analyze**: Read the files in `pkg/controllers/machinesync/`. Identify bottlenecks in the controller logic or inefficiencies in the Ginkgo/Gomega test suite (e.g., redundant setup/teardown, inefficient async polling, slow mock implementations, missing parallelization).
2. **Hypothesize & Implement**: Make a specific modification to the codebase designed to improve performance. Keep changes small and localized to easily identify which modification caused an improvement.
3. **Evaluate**: Run the immutable evaluator from the repository root:
   ```bash
   ./hack/autoresearch/evaluator.py
   ```
4. **Observe & Decide**:
   - Inspect the terminal output and the generated `hack/autoresearch/test-output.log` and `hack/autoresearch/results.tsv`.
   - If the evaluation fails or the duration increases, **revert** your changes (using `git checkout` or similar) and formulate a new hypothesis.
   - If the tests pass and the execution time decreases, **commit** the change to lock in the progress, and begin the next iteration.

## Technical Guidance for OpenShift CAPI Operator
* Consider adjusting test environment configurations or mocking strategies if the tests spend too much time waiting for Kubernetes API server (envtest) responses.
* Review the `AGENTS.md` in the repository root for required testing patterns (e.g., Komega async assertions). Over-polling in tests can add latency.
* Optimize the controller's main reconcile loop or cache access patterns to speed up execution.

**Start your loop now.**
