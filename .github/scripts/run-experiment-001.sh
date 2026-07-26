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

fixture_sha256="$(
  sha256sum fixture/android/app/build/outputs/apk/debug/app-debug.apk |
    cut -d " " -f 1
)"
jq -e \
  --arg fixture_sha256 "${fixture_sha256}" \
  --arg ariadne_revision "${GITHUB_SHA}" '
  (.schema_version == 3) and
  (.target.android_api == 35) and
  (.target.architecture == "x86_64") and
  (.target.package_version_code == 1) and
  (.target.package_sha256 == $fixture_sha256) and
  (.target.ariadne_revision == $ariadne_revision) and
  (.target.ariadne_modified == false) and
  (.artifacts | length == 6) and
  (.comparison.schema_version == 2) and
  (.comparison.unchanged_fields == ["region"]) and
  (.comparison.differences | length == 1) and
  (.comparison.unknowns == []) and
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

failure_dir="${RUNNER_TEMP}/ariadne-failure-proofs"
mkdir "${failure_dir}"

expect_failure() {
  local name="$1"
  local expected_error="$2"
  shift 2

  local stdout="${failure_dir}/${name}.stdout"
  local stderr="${failure_dir}/${name}.stderr"
  if "$@" >"${stdout}" 2>"${stderr}"; then
    echo "${name}: command unexpectedly succeeded" >&2
    exit 1
  fi
  if [[ -s "${stdout}" ]]; then
    echo "${name}: failed command wrote to stdout" >&2
    exit 1
  fi
  if ! grep -F -q "${expected_error}" "${stderr}"; then
    echo "${name}: expected error was not reported" >&2
    exit 1
  fi
  if grep -F -q \
    -e "${baseline_email}" \
    -e "${treatment_email}" \
    -e "private-tamper-marker" \
    "${stderr}"; then
    echo "${name}: failure output disclosed protected input" >&2
    exit 1
  fi
}

expect_failure \
  "missing-package" \
  "ariadne: android check:" \
  "${ariadne}" android check \
  --device emulator-5554 \
  --package dev.ariadne.missing

artifact_dir="${failure_dir}/artifact"
cp -R "${run_dir}" "${artifact_dir}"
rm "${artifact_dir}/evidence.json" "${artifact_dir}/report.md"
printf '\nprivate-tamper-marker\n' >> \
  "${artifact_dir}/baseline/observations/storage.json"
expect_failure \
  "artifact-integrity" \
  "integrity check failed" \
  "${ariadne}" experiment report "${artifact_dir}"
test ! -e "${artifact_dir}/evidence.json"
test ! -e "${artifact_dir}/report.md"

provenance_dir="${failure_dir}/provenance"
cp -R "${run_dir}" "${provenance_dir}"
rm "${provenance_dir}/evidence.json" "${provenance_dir}/report.md"
jq '.package_sha256 = "0000000000000000000000000000000000000000000000000000000000000000"' \
  "${provenance_dir}/treatment/session.json" > \
  "${provenance_dir}/treatment/session.json.tmp"
mv \
  "${provenance_dir}/treatment/session.json.tmp" \
  "${provenance_dir}/treatment/session.json"
expect_failure \
  "provenance-mismatch" \
  "session metadata disagree" \
  "${ariadne}" experiment report "${provenance_dir}"
test ! -e "${provenance_dir}/evidence.json"
test ! -e "${provenance_dir}/report.md"
