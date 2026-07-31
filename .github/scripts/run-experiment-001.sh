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
  (.schema_version == 5) and
  (.manifest_contract_sha256 | test("^[0-9a-f]{64}$")) and
  (.target.android_api == 35) and
  (.target.architecture == "x86_64") and
  (.target.package_version_code == 1) and
  (.target.package_sha256 == $fixture_sha256) and
  (.target.ariadne_revision == $ariadne_revision) and
  (.target.ariadne_modified == false) and
  (.artifacts | length == 6) and
  (.comparison.schema_version == 4) and
  (.comparison.unchanged_fields == ["region"]) and
  (.comparison.normalized_fields == ["request_id"]) and
  (.comparison.differences | length == 1) and
  (.comparison.unknowns == []) and
  (
    .normalizations |
    index("removed declared volatile observation field request_id from comparison") != null
  ) and
  (
    .comparison.differences[0] |
    .field == "variant" and
    .kind == "changed" and
    .baseline == "standard" and
    .treatment == "personalized" and
    .state == "observed"
  )
' "${run_dir}/evidence.json"

contract_digest="$(jq -r '.manifest_contract_sha256' "${run_dir}/evidence.json")"

tap_resource_id="dev.ariadne.fixture:id/observe_button"
for session in baseline treatment; do
  jq -e \
    --arg tap_resource_id "${tap_resource_id}" \
    --arg contract_digest "${contract_digest}" \
    '
    (.schema_version == 6) and
    (.tap_resource_id == $tap_resource_id) and
    (.manifest_contract_sha256 == $contract_digest) and
    (.status == "complete") and
    any(.steps[]; .name == "interact" and .status == "ok" and .exit_code == 0)
    ' "${run_dir}/${session}/session.json"
done

baseline_request_id="$(
  jq -r '.request_id' "${run_dir}/baseline/observations/storage.json"
)"
treatment_request_id="$(
  jq -r '.request_id' "${run_dir}/treatment/observations/storage.json"
)"
test -n "${baseline_request_id}"
test -n "${treatment_request_id}"
test "${baseline_request_id}" != "${treatment_request_id}"
if grep -F -q \
  -e "${baseline_request_id}" \
  -e "${treatment_request_id}" \
  "${run_dir}/evidence.json" \
  "${run_dir}/report.md"; then
  echo "volatile value found in normalized evidence output" >&2
  exit 1
fi
grep -F -q \
  "removed declared volatile observation field request_id from comparison" \
  "${run_dir}/report.md"

baseline_email="$(jq -r '.baseline.email' examples/experiment-001.json)"
treatment_email="$(jq -r '.treatment.email' examples/experiment-001.json)"
failure_dir="${RUNNER_TEMP}/ariadne-failure-proofs"
mkdir "${failure_dir}"

if grep -R -F -q \
  -e "${baseline_email}" \
  -e "${treatment_email}" \
  "${run_dir}"; then
  echo "persona value found in evidence output" >&2
  exit 1
fi

storage_gap_dir=".ariadne/ci/experiment-001-storage-gap"
storage_gap_stdout="${failure_dir}/storage-gap-run.stdout"
storage_gap_stderr="${failure_dir}/storage-gap-run.stderr"
if "${ariadne}" experiment run \
  --device emulator-5554 \
  --package dev.ariadne.fixture \
  --output "${storage_gap_dir}" \
  examples/experiment-001-storage-gap.json \
  >"${storage_gap_stdout}" 2>"${storage_gap_stderr}"; then
  echo "storage gap: experiment run unexpectedly succeeded" >&2
  exit 1
fi
if [[ -s "${storage_gap_stdout}" ]]; then
  echo "storage gap: failed run wrote to stdout" >&2
  exit 1
fi
if ! grep -F -q "capture storage" "${storage_gap_stderr}"; then
  echo "storage gap: expected capture failure was not reported" >&2
  cat "${storage_gap_stderr}" >&2
  exit 1
fi
if grep -F -q \
  -e "${baseline_email}" \
  -e "${treatment_email}" \
  "${storage_gap_stderr}"; then
  echo "storage gap: failure output disclosed a persona value" >&2
  exit 1
fi

test -e "${storage_gap_dir}/baseline/observations/storage.json"
test -e "${storage_gap_dir}/baseline/observations/network.json"
test -e "${storage_gap_dir}/treatment/observations/network.json"
test ! -e "${storage_gap_dir}/treatment/observations/storage.json"
storage_gap_contract_digest="$(jq -r '.manifest_contract_sha256' "${storage_gap_dir}/baseline/session.json")"
jq -e \
  --arg contract_digest "${storage_gap_contract_digest}" \
  '
  (.status == "complete") and
  (.schema_version == 6) and
  (.tap_resource_id == "dev.ariadne.fixture:id/observe_button") and
  (.manifest_contract_sha256 == $contract_digest) and
  any(.steps[]; .name == "interact" and .status == "ok" and .exit_code == 0) and
  (.artifacts | length == 2)
' "${storage_gap_dir}/baseline/session.json"
jq -e \
  --arg contract_digest "${storage_gap_contract_digest}" \
  '
  (.status == "incomplete") and
  (.schema_version == 6) and
  (.tap_resource_id == "dev.ariadne.fixture:id/observe_button") and
  (.manifest_contract_sha256 == $contract_digest) and
  any(.steps[]; .name == "interact" and .status == "ok" and .exit_code == 0) and
  (.failure_stage == "capture_storage") and
  (.artifacts | length == 1) and
  (.artifacts[0].path == "observations/network.json") and
  any(.steps[]; .name == "capture_storage" and .status == "error")
' "${storage_gap_dir}/treatment/session.json"

storage_gap_baseline_request_id="$(
  jq -r '.request_id' \
    "${storage_gap_dir}/baseline/observations/storage.json"
)"
storage_gap_treatment_request_id="$(
  jq -r '.body_base64 | @base64d | fromjson | .request_id' \
    "${storage_gap_dir}/treatment/observations/network.json"
)"
test -n "${storage_gap_baseline_request_id}"
test -n "${storage_gap_treatment_request_id}"
test "${storage_gap_baseline_request_id}" != "${storage_gap_treatment_request_id}"

storage_gap_report_stdout="${failure_dir}/storage-gap-report.stdout"
"${ariadne}" experiment report "${storage_gap_dir}" >"${storage_gap_report_stdout}"
grep -F -x -q "differences: 0" "${storage_gap_report_stdout}"
grep -F -x -q "unknowns: 3" "${storage_gap_report_stdout}"
jq -e \
  --arg contract_digest "${storage_gap_contract_digest}" \
  '
  (.schema_version == 5) and
  (.manifest_contract_sha256 == $contract_digest) and
  (.manifest_name == "experiment-001-email-storage-gap") and
  (.artifacts | length == 5) and
  (.comparison.schema_version == 4) and
  (.comparison.unchanged_fields == []) and
  (.comparison.normalized_fields == []) and
  (.comparison.differences == []) and
  (.comparison.unknowns | map(.field) == ["region", "request_id", "variant"]) and
  all(
    .comparison.unknowns[];
    .state == "unknown" and
    .reason == "treatment storage observation was not captured" and
    (.evidence | length == 3)
  )
' "${storage_gap_dir}/evidence.json"
if grep -F -q \
  -e "standard" \
  -e "personalized" \
  -e "${storage_gap_baseline_request_id}" \
  -e "${storage_gap_treatment_request_id}" \
  "${storage_gap_dir}/evidence.json" \
  "${storage_gap_dir}/report.md"; then
  echo "storage gap: partial report disclosed an observed value" >&2
  exit 1
fi

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
