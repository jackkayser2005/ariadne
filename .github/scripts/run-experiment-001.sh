#!/usr/bin/env bash
set -euo pipefail

run_dir=".ariadne/ci/experiment-001"
ariadne="${RUNNER_TEMP}/ariadne"

adb -s emulator-5554 install -r \
  fixture/android/app/build/outputs/apk/debug/app-debug.apk

"${ariadne}" android check \
  --device emulator-5554 \
  --package dev.ariadne.fixture

"${ariadne}" experiment run \
  --device emulator-5554 \
  --package dev.ariadne.fixture \
  --output "${run_dir}" \
  examples/experiment-001.json

"${ariadne}" experiment report "${run_dir}"

jq -e '
  (.artifacts | length == 6) and
  (.comparison.unchanged_fields == ["region"]) and
  (.comparison.differences | length == 1) and
  (
    .comparison.differences[0] |
    .field == "variant" and
    .baseline == "standard" and
    .treatment == "personalized" and
    .state == "observed"
  )
' "${run_dir}/evidence.json"

baseline_email="$(jq -r '.baseline.email' examples/experiment-001.json)"
treatment_email="$(jq -r '.treatment.email' examples/experiment-001.json)"
if grep -R -F -q \
  -e "${baseline_email}" \
  -e "${treatment_email}" \
  "${run_dir}"; then
  echo "persona value found in evidence output" >&2
  exit 1
fi
