#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# The environment of every test run the lanes start, sourced
# by scripts/test-quick.sh (and so scripts/test-affected.sh) and
# scripts/test-full.sh; the Makefile's test targets set the same.
#
# KTW_ATOMICIO_NO_SYNC=1 (atomicio.NoSyncEnv): atomicio.WriteAtomic skips its
# fsync and durability step. A test's universe lives in a temporary folder no
# power loss can matter to, and the syncs were most of the time of the
# file-writing packages on Windows (the planification store tests: from
# 679 s -> 191 s). The writes stay atomic replaces. internal/core/atomicio's
# own tests clear it and prove the sync path still runs. No product binary
# sets it.
#
# KTW_TEST_TIMEOUT (default 30m): the -timeout every lane passes go test, for
# each package. go test's own default, 10 minutes, failed a package that was
# only slow on a loaded machine; slow is named by the report
# at the end of the run (scripts/test-report.sh), not failed.
export KTW_ATOMICIO_NO_SYNC=1
export KTW_TEST_TIMEOUT="${KTW_TEST_TIMEOUT:-30m}"
