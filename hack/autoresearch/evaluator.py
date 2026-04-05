#!/usr/bin/env python3
"""
Evaluator Script for Autoresearch loop.
This script acts as the "prepare.py" from Karpathy's Autoresearch framework.
It defines the IMMUTABLE EVALUATOR.

It runs the tests for a specific package, measures the execution time,
and extracts the test pass/fail status.
"""

import subprocess
import time
import sys
import re
import os

# Target directory to run tests against
TARGET_DIR = "./pkg/controllers/machinesync/..."

def run_tests():
    start_time = time.time()

    # We must run `make unit` from the root directory to properly invoke setup-envtest
    root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))

    # We capture stdout and stderr, and also write to a log file for the agent to inspect
    log_file = os.path.join(os.path.dirname(__file__), "test-output.log")

    cmd = ["make", "unit", f"TEST_DIRS={TARGET_DIR}"]

    print(f"Running evaluation: {' '.join(cmd)}")
    print(f"Working directory: {root_dir}")

    try:
        process = subprocess.Popen(
            cmd,
            cwd=root_dir,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True
        )

        output = ""
        with open(log_file, "w") as f:
            for line in process.stdout:
                output += line
                f.write(line)
                # Print progress to console as it runs
                sys.stdout.write(line)

        process.wait()
        exit_code = process.returncode
        end_time = time.time()
        duration = end_time - start_time

        return output, exit_code, duration, log_file

    except Exception as e:
        end_time = time.time()
        print(f"Error executing tests: {e}")
        return str(e), 1, end_time - start_time, log_file

def parse_results(output, exit_code, duration, log_file):
    print("=" * 60)
    print("AUTORESEARCH EVALUATION RESULTS")
    print("=" * 60)

    passed_match = re.search(r'([0-9]+) Passed', output)
    failed_match = re.search(r'([0-9]+) Failed', output)

    passed = int(passed_match.group(1)) if passed_match else 0
    failed = int(failed_match.group(1)) if failed_match else 0

    status = "SUCCESS" if exit_code == 0 and failed == 0 else "FAILED"

    print(f"Status:       {status}")
    print(f"Duration:     {duration:.2f} seconds")
    print(f"Tests Passed: {passed}")
    print(f"Tests Failed: {failed}")
    print(f"Log File:     {log_file}")
    print("=" * 60)

    # To facilitate the Autoresearch loop, we output a simple TSV format if needed
    with open(os.path.join(os.path.dirname(__file__), "results.tsv"), "a") as f:
        # timestamp, status, duration, passed, failed
        f.write(f"{time.time()}\t{status}\t{duration:.2f}\t{passed}\t{failed}\n")

    if status == "FAILED":
        print("Evaluation failed. The changes broke the test suite or syntax.")
        sys.exit(1)
    else:
        print("Evaluation passed! A valid performance metric has been recorded.")
        sys.exit(0)

if __name__ == "__main__":
    out, code, time_taken, log_path = run_tests()
    parse_results(out, code, time_taken, log_path)
